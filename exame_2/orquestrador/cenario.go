package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config é tudo o que não muda entre as execuções de uma varredura.
type Config struct {
	ServicoURL    string `json:"servico_url"`
	PrometheusURL string `json:"prometheus_url"`
	ToxiproxyURL  string `json:"toxiproxy_url"`
	Proxy         string `json:"proxy"`
	Saida         string `json:"saida"`
	ScriptK6      string `json:"script_k6"`
	Classes       string `json:"classes"` // "nome:timeout_ms:fração,..."
	FixedDelayMs  int    `json:"fixed_delay_ms"`
	Detector      string `json:"detector_sobrecarga"` // empty | codel (política lifo)
	// Parâmetros das políticas de fila, mandados ao serviço antes de cada
	// execução. Ŝ e W saem da calibração (ver derivarParametros).
	SHatMs           float64 `json:"s_hat_ms"`
	MargemMs         float64 `json:"margem_ms"`
	MaxWaitMs        float64 `json:"max_wait_ms"`
	JanelaMs         float64 `json:"janela_sobrecarga_ms"`
	AtrasoMs         int     `json:"atraso_ms"`
	BackoffBaseMs    int     `json:"backoff_base_ms"`
	BackoffTetoMs    int     `json:"backoff_teto_ms"`
	JitterChegadaMs  int     `json:"jitter_chegada_ms"`
	Fases            Fases   `json:"fases"`
	LimiarRecuperado float64 `json:"limiar_recuperado"`
	JanelaRecupS     int     `json:"janela_recuperacao_s"`
	JanelaFinalS     int     `json:"janela_final_s"`
}

// Cenario é uma execução: uma política de fila, uma política de cliente, uma
// carga, uma repetição.
type Cenario struct {
	ID         string  `json:"id"`
	Fila       string  `json:"politica_fila"` // fifo | timebound | deadline | lifo
	Politica   string  `json:"politica"`      // none | immediate | fixed | backoff
	Seed       int     `json:"seed"`          // números comuns: igual para as 8 configurações de uma célula
	MaxRetries int     `json:"max_retries"`
	CargaPct   float64 `json:"carga_pct"`  // % de μ
	Taxa       float64 `json:"taxa_req_s"` // pedidos novos por segundo
	Rep        int     `json:"rep"`
	SemFalta   bool    `json:"sem_falta"` // true: só carga, sem injeção (validação)
	// Rotulo identifica a execução nas séries do k6 dentro do Prometheus. É o
	// ID mais o instante de início: o mesmo ID pode ser executado mais de uma
	// vez (em varreduras diferentes) e as séries não podem se misturar.
	Rotulo string `json:"rotulo,omitempty"`
}

// Resumo são os números de uma execução que vão para resumo.csv.
type Resumo struct {
	Cenario

	// Regime (antes do gatilho)
	RegimeOriginados   float64 `json:"regime_originados_s"`
	RegimeGoodput      float64 `json:"regime_goodput_s"`
	RegimeRazao        float64 `json:"regime_goodput_sobre_originados"`
	RegimeAmplificacao float64 `json:"regime_amplificacao"`
	RegimeFila         float64 `json:"regime_fila_media"`
	RegimeP99Ms        float64 `json:"regime_p99_ms"`
	RegimeServicoMs    float64 `json:"regime_tempo_servico_ms"` // confere Ŝ na carga real

	// Lei de Little no regime: L medido no servidor contra λ·W medido no cliente.
	LittleL      float64 `json:"little_L_servidor"`
	LittleLambda float64 `json:"little_lambda_cliente"`
	LittleWMs    float64 `json:"little_W_cliente_ms"`
	LittleErro   float64 `json:"little_erro_pct"`

	// Durante o atraso
	AtrasoGoodput      float64 `json:"atraso_goodput_s"`
	AtrasoAmplificacao float64 `json:"atraso_amplificacao"`
	AtrasoFila         float64 `json:"atraso_fila_media"`
	AtrasoServicoMs    float64 `json:"atraso_tempo_servico_ms"`

	// Recuperação (depois que o atraso é removido)
	TempoRecuperacaoS int     `json:"tempo_recuperacao_s"` // -1: não recuperou na janela
	Recuperou         bool    `json:"recuperou"`
	FinalGoodput      float64 `json:"final_goodput_s"`
	FinalRazao        float64 `json:"final_goodput_sobre_regime"`
	FinalFracao       float64 `json:"final_fracao_atendida"` // respostas no prazo ÷ pedidos novos
	FinalAmplificacao float64 `json:"final_amplificacao"`
	FinalFila         float64 `json:"final_fila_media"`
	FinalP99Ms        float64 `json:"final_p99_ms"`
	FinalDesperdicio  float64 `json:"final_desperdicados_s"`
	FinalUteis        float64 `json:"final_uteis_s"`
	FinalRejeitados   float64 `json:"final_rejeitados_s"`
	RecupAmplificacao float64 `json:"recuperacao_amplificacao"`
	Desfecho          string  `json:"desfecho"`
	// Maior congelamento do ambiente durante a execução, medido pelo próprio
	// orquestrador (quanto um sleep de 10 ms passou do ponto).
	MaiorPausaMs float64 `json:"maior_pausa_ambiente_ms"`

	// Por classe de cliente (impatient = timeout curto, patient = longo).
	RegimeFracImp float64 `json:"regime_fracao_impatient"`
	RegimeFracPac float64 `json:"regime_fracao_patient"`
	FinalFracImp  float64 `json:"final_fracao_impatient"`
	FinalFracPac  float64 `json:"final_fracao_patient"`
	TempoRecupImp int     `json:"tempo_recuperacao_impatient_s"`
	TempoRecupPac int     `json:"tempo_recuperacao_patient_s"`
	FinalDespImp  float64 `json:"final_desperdicados_impatient_s"`
	FinalDespPac  float64 `json:"final_desperdicados_patient_s"`
	FinalLatImpMs float64 `json:"final_latencia_ok_impatient_ms"`
	FinalLatPacMs float64 `json:"final_latencia_ok_patient_ms"`
	RegimeInutImp float64 `json:"regime_descartes_desnecessarios_impatient_s"`
	RegimeInutPac float64 `json:"regime_descartes_desnecessarios_patient_s"`
	RecupInutImp  float64 `json:"recuperacao_descartes_desnecessarios_impatient_s"`
	RecupInutPac  float64 `json:"recuperacao_descartes_desnecessarios_patient_s"`

	// Descartes do servidor: taxa no regime (custo sem falta), taxa no final,
	// e totais da execução por motivo.
	RegimeDescartes float64 `json:"regime_descartes_s"`
	FinalDescartes  float64 `json:"final_descartes_s"`
	DescFull        float64 `json:"descartes_full"`
	DescEvicted     float64 `json:"descartes_evicted"`
	DescExpired     float64 `json:"descartes_expired"`
	DescDeadline    float64 `json:"descartes_deadline"`
	DescStale       float64 `json:"descartes_stale"`

	// Fração do tempo com o modo LIFO ligado, por fase (só política lifo).
	RegimeLIFO float64 `json:"regime_lifo_ativo"`
	AtrasoLIFO float64 `json:"atraso_lifo_ativo"`
	RecupLIFO  float64 `json:"recuperacao_lifo_ativo"`
	FinalLIFO  float64 `json:"final_lifo_ativo"`

	IteracoesPerdidas  float64 `json:"iteracoes_descartadas"`
	OutrosErrosCliente float64 `json:"outros_erros_cliente"`
	ErrosServidor      float64 `json:"erros_servidor"`
}

// Manifesto acompanha cada series.csv: diz exatamente o que foi executado.
type Manifesto struct {
	Cenario        Cenario         `json:"cenario"`
	Config         Config          `json:"config"`
	Mu             float64         `json:"mu_req_s"`
	T0             time.Time       `json:"t0"`
	FaltaLigadaS   float64         `json:"falta_ligada_s"`    // offset real, em s desde T0
	FaltaDesligada float64         `json:"falta_desligada_s"` // offset real, em s desde T0
	Fim            time.Time       `json:"fim"`
	ComandoK6      []string        `json:"comando_k6"`
	AmbienteK6     []string        `json:"ambiente_k6"`
	Versoes        Versoes         `json:"versoes"`
	Servico        map[string]any  `json:"servico"`
	TotaisK6       map[string]any  `json:"totais_k6,omitempty"`
	Resumo         Resumo          `json:"resumo"`
	Colunas        []string        `json:"colunas_series_csv"`
	Observacoes    json.RawMessage `json:"observacoes,omitempty"`
}

// Versoes registra o ambiente de software da execução.
type Versoes struct {
	K6        string `json:"k6"`
	Toxiproxy string `json:"toxiproxy"`
	Go        string `json:"go"`
	Kernel    string `json:"kernel"`
	CPUs      int    `json:"cpus"`
	MemoriaMB int    `json:"memoria_mb"`
}

// Series são as séries de 1 s de uma execução. A linha 0 é a linha de base em
// T0; a linha k cobre (k-1, k] segundos depois de T0.
type Series struct {
	N                                                                     int
	Originados, Tentativas, Sucesso, Timeouts, RejCliente, Falhas, Outros []float64
	LatOkMs                                                               []float64
	Recebidos, Rejeitados, Uteis, Desperdicados, Erros                    []float64
	Descartes                                                             []float64 // todos os descartes do servidor, qualquer motivo
	Extras                                                                map[string][]float64
	Area, ServicoSoma, ServicoN                                           []float64
	Fila, Ocupados, NoSistema, VUs, Descartados                           []float64
	P50Ms, P99Ms                                                          []float64
	Limites                                                               []float64
	Buckets                                                               [][]float64 // incrementos por segundo de cada bucket de permanência
}

var colunasCSV = []string{
	"t_s", "fase",
	"originados", "tentativas", "sucesso", "timeouts", "rejeitados_cliente", "falhas",
	"recebidos", "rejeitados", "uteis", "desperdicados", "erros", "descartes",
	"fila", "ocupados", "no_sistema", "p50_ms", "p99_ms", "vus", "descartados",
}

// classesCliente são as classes cujas séries são separadas.
var classesCliente = []string{"impatient", "patient"}

// extra é uma coluna adicional de series.csv: um contador (acumulado e
// diferenciado) ou um medidor instantâneo.
type extra struct {
	Coluna  string
	Seletor string // %s é substituído pelo rótulo da execução nas séries do k6
	K6      bool
	Medidor bool
}

func extras() []extra {
	var out []extra
	for _, c := range classesCliente {
		for _, m := range []string{"originados", "tentativas", "sucesso", "latencia_ok_ms"} {
			out = append(out, extra{Coluna: m + "_" + c, Seletor: `k6_bancada_` + m + `_total{run=%q,class="` + c + `"}`, K6: true})
		}
		out = append(out,
			extra{Coluna: "desperdicados_" + c, Seletor: `svc_requests_completed_total{job="servico",outcome="wasted",class="` + c + `"}`},
			extra{Coluna: "uteis_" + c, Seletor: `svc_requests_completed_total{job="servico",outcome="useful",class="` + c + `"}`},
			extra{Coluna: "desnecessarios_" + c, Seletor: `svc_requests_needless_drops_total{job="servico",class="` + c + `"}`},
		)
	}
	for _, m := range []string{"full", "evicted", "expired", "deadline", "stale"} {
		out = append(out, extra{Coluna: "descartes_" + m, Seletor: `svc_requests_dropped_total{job="servico",reason="` + m + `"}`})
	}
	out = append(out, extra{Coluna: "lifo", Seletor: `svc_lifo_active{job="servico"}`, Medidor: true})
	return out
}

func colunasCompletas() []string {
	cols := append([]string(nil), colunasCSV...)
	for _, e := range extras() {
		cols = append(cols, e.Coluna)
	}
	return cols
}

// vusNecessarios estima quantos VUs o k6 precisa para nunca segurar uma
// chegada: taxa × duração máxima de uma iteração, com folga. Se faltarem VUs
// o k6 descarta iterações e a carga deixa de ser aberta.
func vusNecessarios(cfg Config, c Cenario, respostaMaxMs int) (pre, max int) {
	tentativas := 1
	esperas := 0.0
	if c.Politica != "none" {
		tentativas += c.MaxRetries
	}
	if c.Politica == "backoff" {
		for i := 0; i < c.MaxRetries; i++ {
			esperas += math.Min(float64(cfg.BackoffTetoMs), float64(cfg.BackoffBaseMs)*math.Pow(2, float64(i)))
		}
	}
	if c.Politica == "fixed" {
		esperas = float64(c.MaxRetries * cfg.FixedDelayMs)
	}
	iterS := (float64(cfg.JitterChegadaMs) + float64(tentativas*respostaMaxMs) + esperas) / 1000
	pre = int(math.Ceil(c.Taxa*iterS*1.3)) + 50
	return pre, pre * 2
}

// processoK6 é um k6 em execução.
type processoK6 struct {
	cmd     *exec.Cmd
	fim     chan error // recebe o resultado de cmd.Wait
	Comando []string
	Env     []string
}

// timeoutMedio devolve o timeout médio das classes, ponderado pela fração.
func timeoutMedio(classes string) float64 {
	soma, peso := 0.0, 0.0
	for _, p := range strings.Split(classes, ",") {
		partes := strings.Split(p, ":")
		if len(partes) != 3 {
			continue
		}
		ms, e1 := strconv.ParseFloat(partes[1], 64)
		fr, e2 := strconv.ParseFloat(partes[2], 64)
		if e1 == nil && e2 == nil {
			soma += ms * fr
			peso += fr
		}
	}
	if peso == 0 {
		return math.NaN()
	}
	return soma / peso
}

// derivarParametros completa Ŝ e W a partir da calibração, quando não foram
// fixados por opção: Ŝ = tempo de serviço medido; W = timeout médio − Ŝ −
// margem (a regra do plano, com o Ŝ real da máquina em vez do nominal).
func derivarParametros(cfg *Config, cal Calibracao) {
	if cfg.SHatMs <= 0 && cal.SHatMs > 0 {
		cfg.SHatMs = math.Round(cal.SHatMs*10) / 10
	}
	if cfg.SHatMs <= 0 {
		soma := 0.0
		for _, m := range cal.Medidas {
			soma += m.TempoServicoMs
		}
		if len(cal.Medidas) > 0 {
			cfg.SHatMs = math.Round(soma / float64(len(cal.Medidas)))
		} else {
			cfg.SHatMs = 42
		}
	}
	if cfg.MaxWaitMs <= 0 {
		cfg.MaxWaitMs = math.Round(timeoutMedio(cfg.Classes) - cfg.SHatMs - cfg.MargemMs)
	}
	log.Printf("parâmetros das políticas: Ŝ = %.0f ms, margem = %.0f ms, W = %.0f ms, janela de sobrecarga = %.0f ms, detector %s",
		cfg.SHatMs, cfg.MargemMs, cfg.MaxWaitMs, cfg.JanelaMs, cfg.Detector)
}

// maiorTimeout devolve o maior timeout entre as classes ("nome:ms:fração,...").
func maiorTimeout(classes string) int {
	maior := 0
	for _, p := range strings.Split(classes, ",") {
		partes := strings.Split(p, ":")
		if len(partes) == 3 {
			if ms, err := strconv.Atoi(partes[1]); err == nil && ms > maior {
				maior = ms
			}
		}
	}
	return maior
}

// execK6 monta e inicia o k6. Não espera terminar. classes define os timeouts
// ("nome:ms:fração,..."); respostaMaxMs é o maior tempo que uma tentativa
// pode levar e só serve para dimensionar os VUs.
func execK6(cfg Config, c Cenario, alvo string, duracaoS int, classes string, respostaMaxMs int, dir string) (*processoK6, error) {
	pre, max := vusNecessarios(cfg, c, respostaMaxMs)
	env := []string{
		"TARGET=" + alvo + "/work",
		"RATE=" + strconv.FormatFloat(c.Taxa, 'f', 2, 64),
		"DURATION_S=" + strconv.Itoa(duracaoS),
		"POLICY=" + c.Politica,
		"MAX_RETRIES=" + strconv.Itoa(c.MaxRetries),
		"CLASSES=" + classes,
		"FIXED_DELAY_MS=" + strconv.Itoa(cfg.FixedDelayMs),
		"SEED=" + strconv.Itoa(c.Seed),
		"BACKOFF_BASE_MS=" + strconv.Itoa(cfg.BackoffBaseMs),
		"BACKOFF_CAP_MS=" + strconv.Itoa(cfg.BackoffTetoMs),
		"ARRIVAL_JITTER_MS=" + strconv.Itoa(cfg.JitterChegadaMs),
		"VUS=" + strconv.Itoa(pre),
		"MAX_VUS=" + strconv.Itoa(max),
		"RUN_ID=" + c.rotulo(),
		"SUMMARY=" + filepath.Join(dir, "k6-resumo.json"),
		"K6_PROMETHEUS_RW_SERVER_URL=" + cfg.PrometheusURL + "/api/v1/write",
		"K6_PROMETHEUS_RW_PUSH_INTERVAL=1s",
		"K6_PROMETHEUS_RW_TREND_STATS=avg,p(99)",
		"K6_NO_USAGE_REPORT=true",
	}
	args := []string{
		"run", "--quiet",
		"--out", "experimental-prometheus-rw",
		// Cada timeout vira um aviso; só os erros de verdade vão para o log.
		"--log-output", "file=" + filepath.Join(dir, "k6.log") + ",level=error",
		cfg.ScriptK6,
	}
	cmd := exec.Command("k6", args...)
	cmd.Env = append(os.Environ(), env...)
	saida, err := os.Create(filepath.Join(dir, "k6.saida.txt"))
	if err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = saida, saida
	if err := cmd.Start(); err != nil {
		saida.Close()
		return nil, err
	}
	p := &processoK6{cmd: cmd, fim: make(chan error, 1), Comando: append([]string{"k6"}, args...), Env: env}
	go func() {
		p.fim <- cmd.Wait()
		saida.Close()
	}()
	return p, nil
}

// Esperar espera o k6 sair; mata o processo se passar do limite.
func (p *processoK6) Esperar(limite time.Duration) error {
	select {
	case err := <-p.fim:
		if err != nil {
			return fmt.Errorf("k6 terminou com erro: %w", err)
		}
		return nil
	case <-time.After(limite):
		_ = p.cmd.Process.Kill()
		return fmt.Errorf("k6 não terminou em %v", limite)
	}
}

// Matar interrompe o k6 (usado quando a execução é abandonada por erro).
func (p *processoK6) Matar() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

func dormirAte(t time.Time) {
	if d := time.Until(t); d > 0 {
		time.Sleep(d)
	}
}

// aguardarPrimeiroPedido devolve o instante em que o serviço recebeu o
// primeiro pedido da execução. Esse instante é T0: todas as fases são
// contadas a partir dele, e não do início do processo do k6.
func aguardarPrimeiroPedido(base string, antes float64, limite time.Duration) (time.Time, error) {
	fim := time.Now().Add(limite)
	for time.Now().Before(fim) {
		a, err := lerMetricas(base)
		if err == nil && a.Soma("svc_requests_received_total") > antes {
			return a.Quando, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return time.Time{}, fmt.Errorf("nenhum pedido chegou ao serviço em %v", limite)
}

// rotulo devolve o valor da label `run` das séries do k6. Manifestos antigos
// não têm Rotulo: neles a label era o próprio ID.
func (c Cenario) rotulo() string {
	if c.Rotulo != "" {
		return c.Rotulo
	}
	return c.ID
}

// executarCenario roda uma execução completa e grava series.csv e
// manifesto.json em dir.
func executarCenario(cfg Config, c Cenario, mu float64, dir string) (Resumo, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Resumo{}, err
	}
	c.Rotulo = fmt.Sprintf("%s@%d", c.ID, time.Now().Unix())
	tox := Toxiproxy{URL: cfg.ToxiproxyURL, Proxy: cfg.Proxy}
	if err := tox.Limpar(); err != nil {
		return Resumo{}, err
	}
	if err := aguardarOcioso(cfg.ServicoURL, 60*time.Second); err != nil {
		return Resumo{}, err
	}
	// A política de fila é trocada com o serviço ocioso, sem reiniciá-lo.
	if err := definirPolitica(cfg.ServicoURL, c.Fila, cfg); err != nil {
		return Resumo{}, err
	}
	base, err := lerMetricas(cfg.ServicoURL)
	if err != nil {
		return Resumo{}, err
	}

	f := cfg.Fases
	total := f.Total()
	k6, err := execK6(cfg, c, cfg.ServicoURL, total, cfg.Classes, maiorTimeout(cfg.Classes), dir)
	if err != nil {
		return Resumo{}, err
	}
	concluiu := false
	defer func() {
		if !concluiu {
			k6.Matar()
			_ = tox.Limpar()
		}
	}()

	t0, err := aguardarPrimeiroPedido(cfg.ServicoURL, base.Soma("svc_requests_received_total"), 30*time.Second)
	if err != nil {
		return Resumo{}, err
	}
	seg := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	pausas := vigiarPausas()

	var ligada, desligada float64
	if !c.SemFalta && f.Atraso > 0 {
		dormirAte(seg(f.Aquecimento + f.Regime))
		if err := tox.LigarAtraso(cfg.AtrasoMs); err != nil {
			return Resumo{}, err
		}
		ligada = time.Since(t0).Seconds()
		dormirAte(seg(f.Aquecimento + f.Regime + f.Atraso))
		if err := tox.DesligarAtraso(); err != nil {
			return Resumo{}, err
		}
		desligada = time.Since(t0).Seconds()
	}
	dormirAte(seg(total))
	if err := k6.Esperar(30 * time.Second); err != nil {
		return Resumo{}, err
	}
	concluiu = true
	fim := time.Now()
	// Dá tempo para a última coleta do Prometheus e o último envio do k6.
	time.Sleep(3 * time.Second)
	if err := conferirRelogio(Prometheus{URL: cfg.PrometheusURL}, t0, fim, total); err != nil {
		return Resumo{}, err
	}
	maiorPausa := pausas.Parar()
	if maiorPausa > limitePausa {
		return Resumo{}, fmt.Errorf("o ambiente congelou por %d ms durante a execução (limite %d ms): execução descartada",
			maiorPausa.Milliseconds(), limitePausa.Milliseconds())
	}

	s, err := coletarSeries(Prometheus{URL: cfg.PrometheusURL}, c.rotulo(), t0, total+1)
	if err != nil {
		return Resumo{}, err
	}
	if err := gravarCSV(filepath.Join(dir, "series.csv"), s, f); err != nil {
		return Resumo{}, err
	}
	r := resumir(cfg, c, s)
	r.MaiorPausaMs = float64(maiorPausa.Microseconds()) / 1000

	m := Manifesto{
		Cenario: c, Config: cfg, Mu: mu, T0: t0, Fim: fim,
		FaltaLigadaS: ligada, FaltaDesligada: desligada,
		ComandoK6: k6.Comando, AmbienteK6: k6.Env, Versoes: versoes(tox), Resumo: r, Colunas: colunasCompletas(),
	}
	m.Servico = estadoServico(cfg.ServicoURL)
	m.TotaisK6 = totaisK6(filepath.Join(dir, "k6-resumo.json"))
	if err := gravarJSON(filepath.Join(dir, "manifesto.json"), m); err != nil {
		return Resumo{}, err
	}
	if !c.SemFalta {
		logResumo(r)
	}
	return r, nil
}

// limitePausa é o maior congelamento do ambiente tolerado numa execução. A
// folga de um pedido é timeout − T ≈ 110 ms; um congelamento de 50 ms já
// consome quase metade dela para todos os pedidos em andamento.
const limitePausa = 50 * time.Millisecond

// vigiaPausas mede congelamentos do ambiente: em cada CPU, uma thread fixada
// nela dorme 10 ms em laço e registra quanto cada sono passou do ponto. Se a
// máquina virtual fica sem CPU (o host suspendeu, ou o sistema operacional a
// rebaixou porque a tela apagou), os processos param, inclusive estes laços.
// É uma medida do ambiente, independente do sistema sob teste: serve para
// descartar execuções em que um soluço do laptop, e não o atraso injetado,
// teria sido o gatilho.
//
// Uma thread por CPU, e não uma só: o host pode tirar de cena uma única CPU
// virtual. Uma thread solta só perceberia isso se estivesse justamente nela.
type vigiaPausas struct {
	parar chan struct{}
	wg    sync.WaitGroup
	mu    sync.Mutex
	maior time.Duration
}

func vigiarPausas() *vigiaPausas {
	v := &vigiaPausas{parar: make(chan struct{})}
	for cpu := 0; cpu < runtime.NumCPU(); cpu++ {
		v.wg.Add(1)
		go func(cpu int) {
			defer v.wg.Done()
			// A goroutine fica presa a uma thread, e a thread a uma CPU. A
			// thread é descartada quando a goroutine termina.
			runtime.LockOSThread()
			fixarNaCPU(cpu)
			const passo = 10 * time.Millisecond
			var maior time.Duration
			ultimo := time.Now()
			for {
				select {
				case <-v.parar:
					v.mu.Lock()
					if maior > v.maior {
						v.maior = maior
					}
					v.mu.Unlock()
					return
				default:
				}
				time.Sleep(passo)
				agora := time.Now()
				if excesso := agora.Sub(ultimo) - passo; excesso > maior {
					maior = excesso
				}
				ultimo = agora
			}
		}(cpu)
	}
	return v
}

// Parar encerra a vigia e devolve o maior excesso observado em qualquer CPU.
func (v *vigiaPausas) Parar() time.Duration {
	close(v.parar)
	v.wg.Wait()
	return v.maior
}

// conferirRelogio recusa uma execução em que o tempo não correu como devia.
// Se o host é suspenso no meio (um laptop que dorme, por exemplo), a máquina
// virtual para, o relógio salta e as fases deixam de corresponder ao que foi
// medido. Dois sintomas: a execução dura mais do que o programado, e faltam
// coletas de 1 s no Prometheus.
func conferirRelogio(p Prometheus, t0, fim time.Time, total int) error {
	// O k6 termina até ~6 s depois do fim da carga (jitter + gracefulStop).
	if d := fim.Sub(t0).Seconds(); d > float64(total)+15 {
		return fmt.Errorf("a execução durou %.0f s, mas estava programada para %d s: o host foi suspenso?", d, total)
	}
	v, err := p.Escalar(fmt.Sprintf(`count_over_time(svc_queue_length{job="servico"}[%ds])`, total), t0.Add(time.Duration(total)*time.Second), 1)
	if err != nil {
		return err
	}
	if len(v) != 1 || math.IsNaN(v[0]) || v[0] < 0.97*float64(total) {
		return fmt.Errorf("o Prometheus tem %.0f coletas do serviço em %d s: faltam amostras (host suspenso ou sobrecarregado?)", v[0], total)
	}
	return nil
}

func logResumo(r Resumo) {
	log.Printf("%-28s goodput regime %.1f/s → final %.1f/s (%.0f %% dos pedidos novos), amplificação final %.2f, recuperação %s → %s",
		r.ID, r.RegimeGoodput, r.FinalGoodput, r.FinalFracao*100, r.FinalAmplificacao, textoRecuperacao(r), r.Desfecho)
}

func textoRecuperacao(r Resumo) string {
	if r.TempoRecuperacaoS < 0 {
		return "não"
	}
	return strconv.Itoa(r.TempoRecuperacaoS) + " s"
}

// coletarSeries lê do Prometheus tudo o que foi medido entre T0 e T0+n-1 s.
func coletarSeries(p Prometheus, id string, t0 time.Time, n int) (Series, error) {
	s := Series{N: n}
	var err error
	// Contadores: acumulado interpolado na grade de 1 s e depois diferenciado.
	// zeroAntes vale para os do k6, que nascem com a execução.
	contador := func(dst *[]float64, seletor string, zeroAntes bool) {
		if err != nil {
			return
		}
		var v []float64
		v, err = p.Acumulado(seletor, t0, n, zeroAntes)
		if err == nil {
			*dst = deltas(v)
		}
	}
	// Medidores instantâneos: a última amostra antes de cada instante.
	medidor := func(dst *[]float64, expr string) {
		if err != nil {
			return
		}
		*dst, err = p.Escalar(expr, t0, n)
	}
	k6 := func(nome string) string { return fmt.Sprintf(`k6_%s_total{run=%q}`, nome, id) }
	svc := func(nome string) string { return fmt.Sprintf(`%s{job="servico"}`, nome) }
	desfecho := func(o string) string {
		return fmt.Sprintf(`svc_requests_completed_total{job="servico",outcome=%q}`, o)
	}

	contador(&s.Originados, k6("bancada_originados"), true)
	contador(&s.Tentativas, k6("bancada_tentativas"), true)
	contador(&s.Sucesso, k6("bancada_sucesso"), true)
	contador(&s.Timeouts, k6("bancada_timeouts"), true)
	contador(&s.RejCliente, k6("bancada_rejeitados"), true)
	contador(&s.Falhas, k6("bancada_falhas"), true)
	contador(&s.Outros, k6("bancada_outros_erros"), true)
	contador(&s.LatOkMs, k6("bancada_latencia_ok_ms"), true)
	contador(&s.Descartados, k6("dropped_iterations"), true)
	medidor(&s.VUs, fmt.Sprintf(`sum(k6_vus{run=%q})`, id))

	contador(&s.Recebidos, svc("svc_requests_received_total"), false)
	contador(&s.Rejeitados, `svc_requests_dropped_total{job="servico",reason="full"}`, false)
	contador(&s.Uteis, desfecho("useful"), false)
	contador(&s.Desperdicados, desfecho("wasted"), false)
	contador(&s.Erros, desfecho("error"), false)
	contador(&s.Descartes, svc("svc_requests_dropped_total"), false)
	s.Extras = map[string][]float64{}
	for _, e := range extras() {
		var v []float64
		sel := e.Seletor
		if e.K6 {
			sel = fmt.Sprintf(sel, id)
		}
		if e.Medidor {
			medidor(&v, sel)
		} else {
			contador(&v, sel, e.K6)
		}
		s.Extras[e.Coluna] = v
	}
	contador(&s.Area, svc("svc_in_system_seconds_total"), false)
	contador(&s.ServicoSoma, svc("svc_service_seconds_sum"), false)
	contador(&s.ServicoN, svc("svc_service_seconds_count"), false)
	medidor(&s.Fila, svc("svc_queue_length"))
	medidor(&s.Ocupados, svc("svc_workers_busy"))
	medidor(&s.NoSistema, svc("svc_in_system"))
	if err != nil {
		return s, err
	}

	limites, acumulado, err := p.HistogramaInterpolado(svc("svc_sojourn_seconds_bucket"), t0, n)
	if err != nil {
		return s, err
	}
	s.Limites = limites
	s.Buckets = make([][]float64, len(acumulado))
	for i := range acumulado {
		s.Buckets[i] = deltas(acumulado[i])
	}
	s.P50Ms = make([]float64, n)
	s.P99Ms = make([]float64, n)
	for k := 0; k < n; k++ {
		h := histogramaNoIntervalo(s.Limites, s.Buckets, Intervalo{k, k + 1})
		s.P50Ms[k] = h.quantil(0.50) * 1000
		s.P99Ms[k] = h.quantil(0.99) * 1000
	}
	return s, nil
}

func fmtNum(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ""
	}
	// Duas casas bastam: os incrementos são frações de pedido porque vêm de
	// um acumulado interpolado.
	txt := strconv.FormatFloat(v, 'f', 2, 64)
	if strings.Contains(txt, ".") {
		txt = strings.TrimRight(strings.TrimRight(txt, "0"), ".")
	}
	if txt == "-0" {
		txt = "0"
	}
	return txt
}

func fmtNum1(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ""
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func gravarCSV(caminho string, s Series, f Fases) error {
	arq, err := os.Create(caminho)
	if err != nil {
		return err
	}
	defer arq.Close()
	w := csv.NewWriter(arq)
	if err := w.Write(colunasCompletas()); err != nil {
		return err
	}
	ex := extras()
	for k := 1; k < s.N; k++ {
		linha := []string{
			strconv.Itoa(k), f.FaseDaLinha(k),
			fmtNum(s.Originados[k]), fmtNum(s.Tentativas[k]), fmtNum(s.Sucesso[k]), fmtNum(s.Timeouts[k]),
			fmtNum(s.RejCliente[k]), fmtNum(s.Falhas[k]),
			fmtNum(s.Recebidos[k]), fmtNum(s.Rejeitados[k]), fmtNum(s.Uteis[k]), fmtNum(s.Desperdicados[k]), fmtNum(s.Erros[k]), fmtNum(s.Descartes[k]),
			fmtNum(s.Fila[k]), fmtNum(s.Ocupados[k]), fmtNum(s.NoSistema[k]),
			fmtNum1(s.P50Ms[k]), fmtNum1(s.P99Ms[k]), fmtNum(s.VUs[k]), fmtNum(s.Descartados[k]),
		}
		for _, e := range ex {
			v := s.Extras[e.Coluna]
			if k < len(v) {
				linha = append(linha, fmtNum(v[k]))
			} else {
				linha = append(linha, "")
			}
		}
		if err := w.Write(linha); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// resumir calcula os números de uma execução a partir das séries.
func resumir(cfg Config, c Cenario, s Series) Resumo {
	f := cfg.Fases
	r := Resumo{Cenario: c, TempoRecuperacaoS: -1}
	reg := f.Linhas(FaseRegime)
	atr := f.Linhas(FaseAtraso)
	rec := f.Linhas(FaseRecuperacao)
	fin := Intervalo{rec.Fim - cfg.JanelaFinalS, rec.Fim}
	if fin.Ini < rec.Ini {
		fin.Ini = rec.Ini
	}
	p99 := func(iv Intervalo) float64 {
		return histogramaNoIntervalo(s.Limites, s.Buckets, iv).quantil(0.99) * 1000
	}

	r.RegimeOriginados = media(s.Originados, reg)
	r.RegimeGoodput = media(s.Sucesso, reg)
	r.RegimeRazao = razao(soma(s.Sucesso, reg), soma(s.Originados, reg))
	r.RegimeAmplificacao = razao(soma(s.Recebidos, reg), soma(s.Originados, reg))
	r.RegimeFila = media(s.Fila, reg)
	r.RegimeP99Ms = p99(reg)
	r.RegimeServicoMs = razao(soma(s.ServicoSoma, reg), soma(s.ServicoN, reg)) * 1000

	// Lei de Little: L = λ·W. L vem do servidor (integral exata de N(t));
	// λ e W vêm do cliente (tentativas enviadas e latência das respostas).
	// São dois instrumentos independentes medindo a mesma coisa.
	dur := float64(reg.Fim - reg.Ini)
	r.LittleL = soma(s.Area, reg) / dur
	r.LittleLambda = soma(s.Tentativas, reg) / dur
	r.LittleWMs = razao(soma(s.LatOkMs, reg), soma(s.Sucesso, reg))
	r.LittleErro = razao(r.LittleLambda*r.LittleWMs/1000-r.LittleL, r.LittleL) * 100

	if !c.SemFalta && f.Atraso > 0 {
		r.AtrasoGoodput = media(s.Sucesso, atr)
		r.AtrasoAmplificacao = razao(soma(s.Recebidos, atr), soma(s.Originados, atr))
		r.AtrasoFila = media(s.Fila, atr)
		// Tempo de serviço sob a falta. Os 2 primeiros segundos ficam de fora
		// porque ainda terminam pedidos que começaram antes do atraso, e o
		// último porque a coleta de 1 s que cruza o fim do atraso mistura
		// pedidos lentos e rápidos.
		sob := Intervalo{atr.Ini + 2, atr.Fim - 1}
		if sob.Ini >= sob.Fim {
			sob = atr
		}
		r.AtrasoServicoMs = razao(soma(s.ServicoSoma, sob), soma(s.ServicoN, sob)) * 1000

		// Só faz sentido falar em recuperação se havia um regime para o qual
		// voltar: se o sistema já tinha caído antes do atraso, não há.
		if r.RegimeRazao >= 0.95 {
			d, ok := tempoRecuperacao(s.Sucesso, s.Originados, rec.Ini-1, r.RegimeRazao, cfg.LimiarRecuperado, cfg.JanelaRecupS)
			r.TempoRecuperacaoS, r.Recuperou = d, ok
		}
		r.FinalGoodput = media(s.Sucesso, fin)
		r.FinalRazao = razao(r.FinalGoodput, r.RegimeGoodput)
		r.FinalFracao = fracaoAtendida(s.Sucesso, s.Originados, fin)
		r.FinalAmplificacao = razao(soma(s.Recebidos, fin), soma(s.Originados, fin))
		r.FinalFila = media(s.Fila, fin)
		r.FinalP99Ms = p99(fin)
		r.FinalDesperdicio = media(s.Desperdicados, fin)
		r.FinalUteis = media(s.Uteis, fin)
		r.FinalRejeitados = media(s.Rejeitados, fin)
		r.RecupAmplificacao = razao(soma(s.Recebidos, rec), soma(s.Originados, rec))
		r.Desfecho = classificar(r.RegimeRazao, razao(r.FinalFracao, r.RegimeRazao))
	}
	resumirExtras(cfg, c, s, &r, reg, atr, rec, fin)
	todas := Intervalo{1, s.N}
	r.IteracoesPerdidas = soma(s.Descartados, todas)
	r.OutrosErrosCliente = soma(s.Outros, todas)
	r.ErrosServidor = soma(s.Erros, todas)
	return r
}

// resumirExtras preenche as medidas por classe, por motivo de descarte e do
// modo LIFO.
func resumirExtras(cfg Config, c Cenario, s Series, r *Resumo, reg, atr, rec, fin Intervalo) {
	x := func(col string) []float64 {
		if v, ok := s.Extras[col]; ok {
			return v
		}
		return make([]float64, s.N)
	}
	porClasse := func(cl string) (regFr, finFr float64, tempo int, desp, lat, inutReg, inutRec float64) {
		suc, ori := x("sucesso_"+cl), x("originados_"+cl)
		regFr = fracaoAtendida(suc, ori, reg)
		finFr = fracaoAtendida(suc, ori, fin)
		tempo = -1
		if !c.SemFalta && cfg.Fases.Atraso > 0 && regFr >= 0.95 {
			if d, ok := tempoRecuperacao(suc, ori, rec.Ini-1, regFr, cfg.LimiarRecuperado, cfg.JanelaRecupS); ok {
				tempo = d
			}
		}
		desp = media(x("desperdicados_"+cl), fin)
		lat = razao(soma(x("latencia_ok_ms_"+cl), fin), soma(suc, fin))
		inutReg = media(x("desnecessarios_"+cl), reg)
		inutRec = media(x("desnecessarios_"+cl), rec)
		return
	}
	r.RegimeFracImp, r.FinalFracImp, r.TempoRecupImp, r.FinalDespImp, r.FinalLatImpMs, r.RegimeInutImp, r.RecupInutImp = porClasse("impatient")
	r.RegimeFracPac, r.FinalFracPac, r.TempoRecupPac, r.FinalDespPac, r.FinalLatPacMs, r.RegimeInutPac, r.RecupInutPac = porClasse("patient")

	r.RegimeDescartes = media(s.Descartes, reg)
	r.FinalDescartes = media(s.Descartes, fin)
	todas := Intervalo{1, s.N}
	r.DescFull = soma(x("descartes_full"), todas)
	r.DescEvicted = soma(x("descartes_evicted"), todas)
	r.DescExpired = soma(x("descartes_expired"), todas)
	r.DescDeadline = soma(x("descartes_deadline"), todas)
	r.DescStale = soma(x("descartes_stale"), todas)

	lifo := x("lifo")
	r.RegimeLIFO, r.AtrasoLIFO, r.RecupLIFO, r.FinalLIFO = media(lifo, reg), media(lifo, atr), media(lifo, rec), media(lifo, fin)
}

func gravarJSON(caminho string, v any) error {
	b, err := codificarJSON(v)
	if err != nil {
		return err
	}
	return os.WriteFile(caminho, b, 0o644)
}

func estadoServico(base string) map[string]any {
	resp, err := httpc.Get(base + "/state")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return m
}

// totaisK6 lê do resumo do próprio k6 os totais dos contadores. São uma
// segunda fonte, independente do Prometheus, para conferir as séries.
func totaisK6(caminho string) map[string]any {
	b, err := os.ReadFile(caminho)
	if err != nil {
		return nil
	}
	var d struct {
		Metrics map[string]struct {
			Values map[string]float64 `json:"values"`
		} `json:"metrics"`
	}
	if json.Unmarshal(b, &d) != nil {
		return nil
	}
	out := map[string]any{}
	for nome, m := range d.Metrics {
		if v, ok := m.Values["count"]; ok {
			out[nome] = v
		}
	}
	return out
}
