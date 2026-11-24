package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ------------------------------------------------------------- Calibração

// MedidaMu é uma execução de saturação: carga acima da capacidade, sem retry
// e com timeout longo, para medir quanto o serviço consegue terminar por segundo.
type MedidaMu struct {
	Rep            int     `json:"rep"`
	Mu             float64 `json:"mu_req_s"`
	TempoServicoMs float64 `json:"tempo_servico_ms"`
	JanelaS        float64 `json:"janela_s"`
	Concluidos     float64 `json:"concluidos"`
	Rejeitados     float64 `json:"rejeitados"`
	OcupacaoMedia  float64 `json:"workers_ocupados_media"`
	// Distribuição do tempo de serviço na janela (histograma do serviço) e
	// número de pedidos medidos.
	ServicoN     float64 `json:"tempo_servico_amostras"`
	ServicoP50Ms float64 `json:"tempo_servico_p50_ms"`
	ServicoP95Ms float64 `json:"tempo_servico_p95_ms"`
	ServicoP99Ms float64 `json:"tempo_servico_p99_ms"`
	// μ pela lei de Little na saturação: workers ocupados ÷ tempo de serviço.
	// Tem de bater com o μ contado; se não bater, o tempo de serviço medido
	// não é o que limita a vazão.
	MuLittle float64 `json:"mu_pela_lei_de_little_req_s"`
}

// Calibracao é o resultado de `calibrar`: μ medido e sua reprodutibilidade.
type Calibracao struct {
	Alvo          string     `json:"alvo"`
	Quando        time.Time  `json:"quando"`
	Workers       float64    `json:"workers"`
	Fila          float64    `json:"fila"`
	ServicoMs     float64    `json:"servico_ms"`
	MuNominal     float64    `json:"mu_nominal_req_s"`
	TaxaSaturacao float64    `json:"taxa_saturacao_req_s"`
	Medidas       []MedidaMu `json:"medidas"`
	Mu            float64    `json:"mu_req_s"`
	DesvioMaxPct  float64    `json:"desvio_max_pct"`
	ToleranciaPct float64    `json:"tolerancia_pct"`
	Aprovado      bool       `json:"aprovado"`

	// Ŝ: o tempo de serviço usado pelas políticas de fila (ver derivarParametros).
	SHatMs           float64 `json:"s_hat_ms"`
	SHatDesvioMaxPct float64 `json:"s_hat_desvio_max_pct"`
	SHatP95Ms        float64 `json:"s_hat_p95_ms"`
	ConsistenciaPct  float64 `json:"mu_contado_vs_little_pct"`
	SHatAprovado     bool    `json:"s_hat_aprovado"`
}

// medirMu satura o serviço em `alvo` e mede a vazão de conclusões numa janela.
// A leitura é feita direto em /metrics, nos dois extremos da janela, então
// não depende da resolução do Prometheus.
func medirMu(cfg Config, alvo, id string, taxa float64, aquecS, janelaS int, dir string) (MedidaMu, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return MedidaMu{}, err
	}
	if err := aguardarOcioso(alvo, 60*time.Second); err != nil {
		return MedidaMu{}, err
	}
	base, err := lerMetricas(alvo)
	if err != nil {
		return MedidaMu{}, err
	}
	// A calibração usa sempre a fila fixa: nenhuma política descarta nada.
	if err := definirPolitica(alvo, "fifo", Config{}); err != nil {
		return MedidaMu{}, err
	}
	c := Cenario{ID: id, Fila: "fifo", Politica: "none", Taxa: taxa, Seed: 1}
	// Timeout de 5 s: nenhum pedido aceito é abandonado, então tudo o que o
	// serviço termina é vazão útil.
	// Com a fila cheia a resposta mais lenta leva Q/μ + T ≈ 0,4 s; 1 s de folga.
	k6, err := execK6(cfg, c, alvo, aquecS+janelaS+2, "calibracao:5000:1", 1000, dir)
	if err != nil {
		return MedidaMu{}, err
	}
	defer k6.Matar()
	t0, err := aguardarPrimeiroPedido(alvo, base.Soma("svc_requests_received_total"), 30*time.Second)
	if err != nil {
		return MedidaMu{}, err
	}
	dormirAte(t0.Add(time.Duration(aquecS) * time.Second))
	a, err := lerMetricas(alvo)
	if err != nil {
		return MedidaMu{}, err
	}
	// Ocupação média dos workers na janela, amostrada a cada 100 ms: se não
	// for ≈ C, a carga não saturou o serviço e a medida não é μ.
	var ocup, amostras float64
	fimJanela := t0.Add(time.Duration(aquecS+janelaS) * time.Second)
	for time.Now().Before(fimJanela) {
		if m, err := lerMetricas(alvo); err == nil {
			ocup += m.Valores["svc_workers_busy"]
			amostras++
		}
		time.Sleep(100 * time.Millisecond)
	}
	b, err := lerMetricas(alvo)
	if err != nil {
		return MedidaMu{}, err
	}
	if err := k6.Esperar(40 * time.Second); err != nil {
		return MedidaMu{}, err
	}
	dt := b.Quando.Sub(a.Quando).Seconds()
	if dt > float64(janelaS)+5 {
		return MedidaMu{}, fmt.Errorf("a janela de medição durou %.0f s em vez de %d s: o host foi suspenso?", dt, janelaS)
	}
	n := b.Concluidos() - a.Concluidos()
	m := MedidaMu{
		Mu:             n / dt,
		JanelaS:        dt,
		Concluidos:     n,
		Rejeitados:     b.Soma("svc_requests_dropped_total") - a.Soma("svc_requests_dropped_total"),
		TempoServicoMs: razao(b.Valores["svc_service_seconds_sum"]-a.Valores["svc_service_seconds_sum"], b.Valores["svc_service_seconds_count"]-a.Valores["svc_service_seconds_count"]) * 1000,
		OcupacaoMedia:  razao(ocup, amostras),
	}
	m.ServicoN = b.Valores["svc_service_seconds_count"] - a.Valores["svc_service_seconds_count"]
	h := histogramaEntre(a, b, "svc_service_seconds_bucket")
	m.ServicoP50Ms, m.ServicoP95Ms, m.ServicoP99Ms = h.quantil(0.50)*1000, h.quantil(0.95)*1000, h.quantil(0.99)*1000
	m.MuLittle = razao(m.OcupacaoMedia*1000, m.TempoServicoMs)
	return m, nil
}

// calibrar mede μ `reps` vezes e confere a tolerância de ±5 %.
func calibrar(cfg Config, alvo, nome string, reps, janelaS int) (Calibracao, error) {
	est, err := lerMetricas(alvo)
	if err != nil {
		return Calibracao{}, err
	}
	cal := Calibracao{
		Alvo: alvo, Quando: time.Now(),
		Workers:       est.Valores["svc_config_workers"],
		Fila:          est.Valores["svc_config_queue"],
		ServicoMs:     est.Valores["svc_config_service_ms"],
		ToleranciaPct: 5,
	}
	cal.MuNominal = cal.Workers * 1000 / cal.ServicoMs
	// 1,5 × o nominal: o suficiente para manter os C workers ocupados o tempo
	// todo, sem gastar CPU do laptop rejeitando pedidos.
	cal.TaxaSaturacao = 1.5 * cal.MuNominal
	var mus []float64
	for rep := 1; rep <= reps; rep++ {
		id := fmt.Sprintf("%s-rep%d", nome, rep)
		m, err := medirMu(cfg, alvo, id, cal.TaxaSaturacao, 5, janelaS, filepath.Join(cfg.Saida, nome, id))
		if err != nil {
			return cal, err
		}
		m.Rep = rep
		cal.Medidas = append(cal.Medidas, m)
		mus = append(mus, m.Mu)
		log.Printf("%s: μ = %.2f req/s (tempo de serviço %.2f ms, ocupação média %.2f de %.0f workers)",
			id, m.Mu, m.TempoServicoMs, m.OcupacaoMedia, cal.Workers)
	}
	cal.Mu, cal.DesvioMaxPct = desvioMaxPct(mus)
	cal.Aprovado = cal.DesvioMaxPct <= cal.ToleranciaPct
	log.Printf("%s: μ = %.2f req/s (nominal %.0f), desvio máximo %.2f %% → %s",
		nome, cal.Mu, cal.MuNominal, cal.DesvioMaxPct, aprovado(cal.Aprovado))
	avaliarSHat(&cal)
	log.Printf("%s: Ŝ = %.2f ms (p95 %.1f ms), desvio máximo entre repetições %.2f %%, μ contado vs C/Ŝ %.2f %% → %s",
		nome, cal.SHatMs, cal.SHatP95Ms, cal.SHatDesvioMaxPct, cal.ConsistenciaPct, aprovado(cal.SHatAprovado))
	cal.Aprovado = cal.Aprovado && cal.SHatAprovado
	return cal, gravarJSON(filepath.Join(cfg.Saida, nome+".json"), cal)
}

// avaliarSHat calcula Ŝ, o tempo médio que o serviço leva para processar um
// pedido, com duas checagens de confiabilidade: as repetições concordam
// (±5 %) e o tempo medido explica a vazão contada (lei de Little na
// saturação: μ = workers ocupados ÷ Ŝ, ±3 %).
func avaliarSHat(cal *Calibracao) {
	var ts, p95 []float64
	pesoT, somaT := 0.0, 0.0
	consist := 0.0
	for _, m := range cal.Medidas {
		ts = append(ts, m.TempoServicoMs)
		p95 = append(p95, m.ServicoP95Ms)
		somaT += m.TempoServicoMs * m.ServicoN
		pesoT += m.ServicoN
		if d := math.Abs(m.MuLittle-m.Mu) / m.Mu * 100; d > consist {
			consist = d
		}
	}
	if len(ts) == 0 || pesoT == 0 {
		cal.SHatMs, cal.SHatAprovado = math.NaN(), false
		return
	}
	// Média ponderada pelo número de pedidos de cada repetição.
	cal.SHatMs = somaT / pesoT
	_, cal.SHatDesvioMaxPct = desvioMaxPct(ts)
	cal.SHatP95Ms, _ = desvioMaxPct(p95)
	cal.ConsistenciaPct = consist
	cal.SHatAprovado = cal.SHatDesvioMaxPct <= 5 && cal.ConsistenciaPct <= 3
}

// histogramaEntre monta, a partir de duas leituras de /metrics, o histograma
// das observações feitas entre elas.
func histogramaEntre(a, b Amostra, nome string) Histograma {
	type par struct{ le, n float64 }
	var ps []par
	for k, vb := range b.Valores {
		if !strings.HasPrefix(k, nome+"{") {
			continue
		}
		i := strings.Index(k, `le="`)
		if i < 0 {
			continue
		}
		resto := k[i+4:]
		txt := resto[:strings.IndexByte(resto, '"')]
		le, err := strconv.ParseFloat(txt, 64)
		if txt == "+Inf" {
			le, err = math.Inf(1), nil
		}
		if err != nil {
			continue
		}
		ps = append(ps, par{le, vb - a.Valores[k]})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].le < ps[j].le })
	h := Histograma{}
	for _, p := range ps {
		h.Limites = append(h.Limites, p.le)
		h.Contagens = append(h.Contagens, p.n)
	}
	return h
}

func aprovado(ok bool) string {
	if ok {
		return "APROVADO"
	}
	return "REPROVADO"
}

func lerCalibracao(cfg Config) (Calibracao, error) {
	var cal Calibracao
	b, err := os.ReadFile(filepath.Join(cfg.Saida, "calibracao.json"))
	if err != nil {
		return cal, err
	}
	return cal, json.Unmarshal(b, &cal)
}

// -------------------------------------------------------------- Varredura

// PlanoVarredura descreve uma varredura: o produto cartesiano de políticas,
// número de retries, cargas e repetições.
type PlanoVarredura struct {
	Nome      string    `json:"nome"`
	Filas     []string  `json:"politicas_fila"`
	Politicas []string  `json:"politicas"`
	Retries   []int     `json:"retries"`
	Cargas    []float64 `json:"cargas_pct"`
	Reps      int       `json:"repeticoes"`
	Semente   int64     `json:"semente"`
}

// ManifestoVarredura acompanha resumo.csv.
type ManifestoVarredura struct {
	Plano       PlanoVarredura `json:"plano"`
	Config      Config         `json:"config"`
	Calibracao  Calibracao     `json:"calibracao"`
	Versoes     Versoes        `json:"versoes"`
	Inicio      time.Time      `json:"inicio"`
	Fim         time.Time      `json:"fim"`
	Ordem       []string       `json:"ordem_de_execucao"`
	Execucoes   int            `json:"execucoes"`
	Reaproveita int            `json:"execucoes_reaproveitadas"`
	Descartadas []string       `json:"execucoes_descartadas"` // tentativas recusadas pelas checagens de ambiente
}

func idCenario(fila, politica string, retries int, carga float64, rep int) string {
	if politica == "none" {
		return fmt.Sprintf("%s-none-c%02.0f-rep%d", fila, carga, rep)
	}
	return fmt.Sprintf("%s-%s-r%d-c%02.0f-rep%d", fila, politica, retries, carga, rep)
}

// sementeCelula define os números comuns: a mesma para todas as
// configurações de uma (carga, repetição).
func sementeCelula(carga float64, rep int) int { return rep*1000 + int(math.Round(carga*10)) }

// cenarios monta a lista de execuções. A ordem é embaralhada dentro de cada
// repetição (com semente fixa), para que uma deriva lenta do ambiente não se
// confunda com o efeito de uma política ou de uma carga.
func (p PlanoVarredura) cenarios(mu float64) []Cenario {
	rng := rand.New(rand.NewSource(p.Semente))
	var todos []Cenario
	for rep := 1; rep <= p.Reps; rep++ {
		var bloco []Cenario
		for _, fila := range p.Filas {
			for _, pol := range p.Politicas {
				retries := p.Retries
				if pol == "none" {
					retries = []int{0} // sem retry não há o que variar
				}
				for _, r := range retries {
					for _, carga := range p.Cargas {
						bloco = append(bloco, Cenario{
							ID: idCenario(fila, pol, r, carga, rep), Fila: fila, Politica: pol, MaxRetries: r,
							CargaPct: carga, Taxa: mu * carga / 100, Rep: rep, Seed: sementeCelula(carga, rep),
						})
					}
				}
			}
		}
		rng.Shuffle(len(bloco), func(i, j int) { bloco[i], bloco[j] = bloco[j], bloco[i] })
		todos = append(todos, bloco...)
	}
	return todos
}

func varrer(cfg Config, cal Calibracao, plano PlanoVarredura) error {
	raiz := filepath.Join(cfg.Saida, plano.Nome)
	if err := os.MkdirAll(raiz, 0o755); err != nil {
		return err
	}
	lista := plano.cenarios(cal.Mu)
	tox := Toxiproxy{URL: cfg.ToxiproxyURL, Proxy: cfg.Proxy}
	man := ManifestoVarredura{Plano: plano, Config: cfg, Calibracao: cal, Versoes: versoes(tox), Inicio: time.Now()}
	porExec := time.Duration(cfg.Fases.Total()+12) * time.Second
	log.Printf("varredura %q: %d execuções, cerca de %s", plano.Nome, len(lista), (time.Duration(len(lista)) * porExec).Round(time.Minute))

	var resumos []Resumo
	for i, c := range lista {
		dir := filepath.Join(raiz, "execucoes", c.ID)
		man.Ordem = append(man.Ordem, c.ID)
		if r, ok := lerResumo(filepath.Join(dir, "manifesto.json")); ok {
			log.Printf("[%d/%d] %s já executado; reaproveitando", i+1, len(lista), c.ID)
			resumos = append(resumos, r)
			man.Reaproveita++
			continue
		}
		log.Printf("[%d/%d] %s: %.1f req/s (%.0f %% de μ)", i+1, len(lista), c.ID, c.Taxa, c.CargaPct)
		// Até quatro tentativas: uma execução descartada por congelamento do
		// ambiente não deve derrubar horas de varredura, mas um ambiente que
		// não se estabiliza deve.
		var r Resumo
		var err error
		for tentativa := 1; ; tentativa++ {
			if r, err = executarCenario(cfg, c, cal.Mu, dir); err == nil {
				break
			}
			_ = os.RemoveAll(dir)
			man.Descartadas = append(man.Descartadas, fmt.Sprintf("%s: %v", c.ID, err))
			if tentativa >= 4 {
				return fmt.Errorf("%s: %w", c.ID, err)
			}
			log.Printf("%s descartada (%v); repetindo (tentativa %d de 4)", c.ID, err, tentativa+1)
			time.Sleep(10 * time.Second)
		}
		resumos = append(resumos, r)
		man.Execucoes++
		if err := gravarResumoCSV(filepath.Join(raiz, "resumo.csv"), resumos); err != nil {
			return err
		}
	}
	man.Fim = time.Now()
	// O resumo final cobre tudo o que existe no diretório, não só este plano:
	// uma varredura pode ser ampliada em chamadas sucessivas com o mesmo nome.
	if err := gravarResumoCSV(filepath.Join(raiz, "resumo.csv"), resumosDoDiretorio(raiz)); err != nil {
		return err
	}
	return gravarJSON(filepath.Join(raiz, fmt.Sprintf("manifesto-%s.json", man.Inicio.UTC().Format("20060102T150405Z"))), man)
}

// resumosDoDiretorio lê o resumo de todas as execuções gravadas em raiz.
func resumosDoDiretorio(raiz string) []Resumo {
	arquivos, _ := filepath.Glob(filepath.Join(raiz, "execucoes", "*", "manifesto.json"))
	var rs []Resumo
	for _, arq := range arquivos {
		if r, ok := lerResumo(arq); ok {
			rs = append(rs, r)
		}
	}
	return rs
}

// reprocessar refaz series.csv, o resumo de cada manifesto e resumo.csv de
// uma varredura já executada, relendo as séries do Prometheus a partir do T0
// gravado em cada manifesto. Serve para aplicar uma mudança nas métricas
// derivadas sem repetir as execuções. Só funciona enquanto o Prometheus ainda
// guarda os dados (retenção de 2 dias, e o container não pode ter sido removido).
func reprocessar(cfg Config, nome string) error {
	raiz := filepath.Join(cfg.Saida, nome)
	arquivos, err := filepath.Glob(filepath.Join(raiz, "execucoes", "*", "manifesto.json"))
	if err != nil {
		return err
	}
	if len(arquivos) == 0 {
		return fmt.Errorf("nenhuma execução em %s", raiz)
	}
	prom := Prometheus{URL: cfg.PrometheusURL}
	var resumos []Resumo
	for _, arq := range arquivos {
		b, err := os.ReadFile(arq)
		if err != nil {
			return err
		}
		var m Manifesto
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("%s: %w", arq, err)
		}
		// As definições das métricas derivadas são as do código atual; o que
		// foi executado (fases, timeout, atraso) continua sendo o do manifesto.
		m.Config.LimiarRecuperado, m.Config.JanelaRecupS, m.Config.JanelaFinalS = cfg.LimiarRecuperado, cfg.JanelaRecupS, cfg.JanelaFinalS
		s, err := coletarSeries(prom, m.Cenario.rotulo(), m.T0, m.Config.Fases.Total()+1)
		if err != nil {
			return fmt.Errorf("%s: %w", m.Cenario.ID, err)
		}
		if soma(s.Originados, Intervalo{1, s.N}) == 0 || soma(s.Recebidos, Intervalo{1, s.N}) == 0 {
			return fmt.Errorf("%s: o Prometheus não tem mais as séries desta execução", m.Cenario.ID)
		}
		dir := filepath.Dir(arq)
		if err := gravarCSV(filepath.Join(dir, "series.csv"), s, m.Config.Fases); err != nil {
			return err
		}
		pausa := m.Resumo.MaiorPausaMs // medida na hora; não dá para refazer
		m.Resumo = resumir(m.Config, m.Cenario, s)
		m.Resumo.MaiorPausaMs = pausa
		m.Colunas = colunasCSV
		if err := gravarJSON(arq, m); err != nil {
			return err
		}
		logResumo(m.Resumo)
		resumos = append(resumos, m.Resumo)
	}
	return gravarResumoCSV(filepath.Join(raiz, "resumo.csv"), resumos)
}

// lerResumo recupera o resumo de uma execução já gravada. Os null do JSON
// (razões indefinidas) voltam a ser NaN.
func lerResumo(caminho string) (Resumo, bool) {
	b, err := os.ReadFile(caminho)
	if err != nil {
		return Resumo{}, false
	}
	var m struct {
		Resumo Resumo `json:"resumo"`
	}
	var bruto struct {
		Resumo map[string]any `json:"resumo"`
	}
	if json.Unmarshal(b, &m) != nil || json.Unmarshal(b, &bruto) != nil || m.Resumo.ID == "" {
		return Resumo{}, false
	}
	v := reflect.ValueOf(&m.Resumo).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		nome, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if t.Field(i).Type.Kind() != reflect.Float64 || nome == "" {
			continue
		}
		if val, existe := bruto.Resumo[nome]; existe && val == nil {
			v.Field(i).SetFloat(math.NaN())
		}
	}
	return m.Resumo, true
}

// camposResumo lista, na ordem da struct, os campos de Resumo que vão para
// resumo.csv (os de Cenario primeiro), com o nome da coluna = tag json.
func camposResumo() (nomes []string, caminhos [][]int) {
	var visitar func(t reflect.Type, prefixo []int)
	visitar = func(t reflect.Type, prefixo []int) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			caminho := append(append([]int(nil), prefixo...), i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				visitar(f.Type, caminho)
				continue
			}
			nome, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if nome == "" || nome == "-" || nome == "rotulo" {
				continue
			}
			nomes = append(nomes, nome)
			caminhos = append(caminhos, caminho)
		}
	}
	visitar(reflect.TypeOf(Resumo{}), nil)
	return
}

func gravarResumoCSV(caminho string, rs []Resumo) error {
	ordenado := append([]Resumo(nil), rs...)
	ordemFila := map[string]int{"fifo": 0, "timebound": 1, "deadline": 2, "lifo": 3}
	ordemPol := map[string]int{"none": 0, "immediate": 1, "fixed": 2, "backoff": 3}
	sort.SliceStable(ordenado, func(i, j int) bool {
		a, b := ordenado[i], ordenado[j]
		if a.Fila != b.Fila {
			return ordemFila[a.Fila] < ordemFila[b.Fila]
		}
		if a.Politica != b.Politica {
			return ordemPol[a.Politica] < ordemPol[b.Politica]
		}
		if a.MaxRetries != b.MaxRetries {
			return a.MaxRetries < b.MaxRetries
		}
		if a.CargaPct != b.CargaPct {
			return a.CargaPct < b.CargaPct
		}
		return a.Rep < b.Rep
	})
	arq, err := os.Create(caminho)
	if err != nil {
		return err
	}
	defer arq.Close()
	w := csv.NewWriter(arq)
	nomes, caminhos := camposResumo()
	if err := w.Write(nomes); err != nil {
		return err
	}
	for _, r := range ordenado {
		v := reflect.ValueOf(r)
		linha := make([]string, len(caminhos))
		for i, c := range caminhos {
			campo := v.FieldByIndex(c)
			switch campo.Kind() {
			case reflect.Float64:
				if f := campo.Float(); !math.IsNaN(f) && !math.IsInf(f, 0) {
					linha[i] = strconv.FormatFloat(f, 'f', 3, 64)
				}
			case reflect.Int:
				linha[i] = strconv.Itoa(int(campo.Int()))
			case reflect.Bool:
				linha[i] = strconv.FormatBool(campo.Bool())
			default:
				linha[i] = fmt.Sprint(campo.Interface())
			}
		}
		if err := w.Write(linha); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// ---------------------------------------------------------------- Ambiente

func versoes(tox Toxiproxy) Versoes {
	v := Versoes{Go: runtime.Version(), CPUs: runtime.NumCPU(), Toxiproxy: tox.Versao()}
	if out, err := exec.Command("k6", "version").Output(); err == nil {
		v.K6 = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		v.Kernel = strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, linha := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(linha, "MemTotal:") {
				campos := strings.Fields(linha)
				if len(campos) >= 2 {
					kb, _ := strconv.Atoi(campos[1])
					v.MemoriaMB = kb / 1024
				}
			}
		}
	}
	return v
}
