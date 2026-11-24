// Orquestrador da bancada de falhas metaestáveis (Lab Exame CSC-27).
//
// Liga e desliga o atraso no Toxiproxy, roda o k6, lê as séries do Prometheus
// e exporta CSV com manifesto. Subcomandos:
//
//	calibrar    mede μ (3 execuções de saturação) e confere ±5 %
//	cenario     uma execução: uma política, uma carga
//	varredura   produto cartesiano políticas × retries × cargas × repetições
//	tudo        calibrar + varredura principal do experimento 2
//	reprocessar refaz CSV e resumos de uma varredura já executada
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func env(nome, padrao string) string {
	if v := os.Getenv(nome); v != "" {
		return v
	}
	return padrao
}

func envInt(nome string, padrao int) int {
	if v := os.Getenv(nome); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("variável %s inválida: %q", nome, v)
		}
		return n
	}
	return padrao
}

// flagsComuns registra os parâmetros que valem para qualquer subcomando. Os
// padrões são os da proposta e podem vir do ambiente (docker-compose.yml).
func flagsComuns(fs *flag.FlagSet) *Config {
	c := &Config{
		ServicoURL:       env("SERVICE_URL", "http://servico:8080"),
		PrometheusURL:    env("PROM_URL", "http://prometheus:9090"),
		ToxiproxyURL:     env("TOXIPROXY_URL", "http://toxiproxy:8474"),
		Proxy:            env("PROXY_NAME", "pg"),
		ScriptK6:         env("K6_SCRIPT", "/carga/script.js"),
		LimiarRecuperado: 0.95,
		JanelaRecupS:     5,
		JanelaFinalS:     30,
	}
	fs.StringVar(&c.Saida, "saida", env("OUT", "/resultados"), "diretório de resultados")
	fs.StringVar(&c.Classes, "classes", env("CLASSES", "impatient:100:0.5,patient:200:0.5"), "classes de cliente: nome:timeout_ms:fração,...")
	fs.IntVar(&c.FixedDelayMs, "fixed-delay", envInt("FIXED_DELAY_MS", 50), "espera antes de cada retry na política fixed, em ms")
	fs.StringVar(&c.Detector, "detector", env("OVERLOAD_DETECTOR", "empty"), "detector de sobrecarga da política lifo: empty | codel")
	fs.Float64Var(&c.SHatMs, "s-hat", 0, "Ŝ, tempo de serviço esperado, em ms (0 = medido na calibração)")
	fs.Float64Var(&c.MargemMs, "margem", 10, "margem das políticas de tempo, em ms")
	fs.Float64Var(&c.MaxWaitMs, "max-wait", 0, "W das políticas timebound e lifo, em ms (0 = timeout médio − Ŝ − margem)")
	fs.Float64Var(&c.JanelaMs, "janela", 100, "janela do detector de sobrecarga, em ms")
	fs.IntVar(&c.AtrasoMs, "atraso-ms", envInt("DELAY_MS", 200), "latência injetada no banco, em ms")
	fs.IntVar(&c.BackoffBaseMs, "backoff-base", envInt("BACKOFF_BASE_MS", 100), "base do backoff exponencial, em ms")
	fs.IntVar(&c.BackoffTetoMs, "backoff-teto", envInt("BACKOFF_CAP_MS", 1000), "teto do backoff, em ms")
	fs.IntVar(&c.JitterChegadaMs, "jitter-chegada", envInt("ARRIVAL_JITTER_MS", 1000), "espalhamento aleatório das chegadas, em ms (0 = chegadas igualmente espaçadas)")
	fs.IntVar(&c.Fases.Aquecimento, "aquecimento", envInt("WARMUP_S", 10), "fase de aquecimento, em s")
	fs.IntVar(&c.Fases.Regime, "regime", envInt("REGIME_S", 30), "fase de regime, em s")
	fs.IntVar(&c.Fases.Atraso, "atraso", envInt("FAULT_S", 20), "duração do atraso injetado, em s")
	fs.IntVar(&c.Fases.Recuperacao, "recuperacao", envInt("RECOVERY_S", 90), "fase de recuperação, em s")
	return c
}

func listaFloat(s string) []float64 {
	var out []float64
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			log.Fatalf("número inválido: %q", p)
		}
		out = append(out, v)
	}
	return out
}

func listaInt(s string) []int {
	var out []int
	for _, v := range listaFloat(s) {
		out = append(out, int(v))
	}
	return out
}

func listaPoliticas(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		switch p {
		case "none", "immediate", "fixed", "backoff":
			out = append(out, p)
		case "":
		default:
			log.Fatalf("política inválida: %q (use none, immediate, fixed ou backoff)", p)
		}
	}
	return out
}

func listaFilas(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		switch p {
		case "fifo", "timebound", "deadline", "lifo":
			out = append(out, p)
		case "":
		default:
			log.Fatalf("política de fila inválida: %q (use fifo, timebound, deadline ou lifo)", p)
		}
	}
	return out
}

// esperarBancada só devolve quando serviço, proxy e Prometheus respondem.
func esperarBancada(cfg Config) {
	for _, u := range []string{
		cfg.ServicoURL + "/healthz",
		cfg.PrometheusURL + "/-/ready",
		cfg.ToxiproxyURL + "/version",
	} {
		if err := aguardarPronto(u, 90*time.Second); err != nil {
			log.Fatal(err)
		}
	}
	// O Prometheus precisa ter pelo menos uma coleta do serviço.
	limite := time.Now().Add(30 * time.Second)
	p := Prometheus{URL: cfg.PrometheusURL}
	for time.Now().Before(limite) {
		v, err := p.Escalar(`sum(svc_requests_received_total{job="servico"})`, time.Now(), 1)
		if err == nil && len(v) == 1 && v[0] == v[0] {
			return
		}
		time.Sleep(time.Second)
	}
	log.Fatal("o Prometheus não está coletando o serviço")
}

// calibracaoOuCalibrar reaproveita calibracao.json se existir.
func calibracaoOuCalibrar(cfg Config) Calibracao {
	if cal, err := lerCalibracao(cfg); err == nil && cal.Mu > 0 {
		log.Printf("usando calibração existente: μ = %.2f req/s (%s)", cal.Mu, cal.Quando.Format(time.RFC3339))
		return cal
	}
	cal, err := calibrar(cfg, cfg.ServicoURL, "calibracao", 3, 30)
	if err != nil {
		log.Fatal(err)
	}
	return cal
}

func uso() {
	fmt.Fprint(os.Stderr, `uso: orquestrador <subcomando> [opções]

  calibrar     mede μ (3 execuções de saturação) e confere ±5 %
  cenario      uma execução: -fila, -politica, -carga, -retries, -rep
  varredura    -nome, -filas, -politicas, -retries, -cargas, -reps
  tudo         calibrar + varredura principal (4 filas × 2 clientes × 4 cargas × 3 repetições)
  reprocessar  refaz CSV e resumos de uma varredura a partir do Prometheus: -nome

Use "orquestrador <subcomando> -h" para ver as opções.
`)
	os.Exit(2)
}

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 {
		uso()
	}
	sub, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	cfg := flagsComuns(fs)

	switch sub {
	case "calibrar":
		_ = fs.Parse(args)
		esperarBancada(*cfg)
		cal, err := calibrar(*cfg, cfg.ServicoURL, "calibracao", 3, 30)
		if err != nil {
			log.Fatal(err)
		}
		if !cal.Aprovado {
			log.Fatalf("μ não é reprodutível dentro de ±%.0f %%", cal.ToleranciaPct)
		}

	case "cenario":
		fila := fs.String("fila", "fifo", "fifo | timebound | deadline | lifo")
		pol := fs.String("politica", "fixed", "none | immediate | fixed | backoff")
		carga := fs.Float64("carga", 80, "carga, em % de μ")
		retries := fs.Int("retries", envInt("MAX_RETRIES", 3), "tentativas extras por pedido")
		rep := fs.Int("rep", 1, "número da repetição")
		nome := fs.String("nome", "avulso", "subdiretório de resultados")
		_ = fs.Parse(args)
		esperarBancada(*cfg)
		cal := calibracaoOuCalibrar(*cfg)
		derivarParametros(cfg, cal)
		p, f := listaPoliticas(*pol), listaFilas(*fila)
		if len(p) != 1 || len(f) != 1 {
			log.Fatal("informe exatamente uma política de fila e uma de cliente")
		}
		c := Cenario{
			ID: idCenario(f[0], p[0], *retries, *carga, *rep), Fila: f[0], Politica: p[0], MaxRetries: *retries,
			CargaPct: *carga, Taxa: cal.Mu * *carga / 100, Rep: *rep, Seed: sementeCelula(*carga, *rep),
		}
		if p[0] == "none" {
			c.MaxRetries = 0
		}
		if _, err := executarCenario(*cfg, c, cal.Mu, filepath.Join(cfg.Saida, *nome, "execucoes", c.ID)); err != nil {
			log.Fatal(err)
		}

	case "varredura", "tudo":
		nome := fs.String("nome", "principal", "nome da varredura (subdiretório de resultados)")
		filas := fs.String("filas", env("QUEUE_POLICIES", "fifo,timebound,deadline,lifo"), "políticas de fila")
		pols := fs.String("politicas", env("POLICIES", "fixed,backoff"), "políticas de retry")
		retries := fs.String("retries", env("MAX_RETRIES", "3"), "tentativas extras por pedido (lista)")
		cargas := fs.String("cargas", env("LOADS", "20,50,70,90"), "cargas, em % de μ")
		reps := fs.Int("reps", envInt("REPS", 3), "repetições por combinação")
		semente := fs.Int64("semente", 27, "semente do embaralhamento da ordem de execução")
		_ = fs.Parse(args)
		esperarBancada(*cfg)
		var cal Calibracao
		if sub == "tudo" {
			var err error
			if cal, err = calibrar(*cfg, cfg.ServicoURL, "calibracao", 3, 30); err != nil {
				log.Fatal(err)
			}
			if !cal.Aprovado {
				log.Fatalf("μ não é reprodutível dentro de ±%.0f %%", cal.ToleranciaPct)
			}
		} else {
			cal = calibracaoOuCalibrar(*cfg)
		}
		derivarParametros(cfg, cal)
		plano := PlanoVarredura{
			Nome: *nome, Filas: listaFilas(*filas), Politicas: listaPoliticas(*pols), Retries: listaInt(*retries),
			Cargas: listaFloat(*cargas), Reps: *reps, Semente: *semente,
		}
		if err := varrer(*cfg, cal, plano); err != nil {
			log.Fatal(err)
		}

	case "reprocessar":
		nome := fs.String("nome", "principal", "nome da varredura")
		_ = fs.Parse(args)
		esperarBancada(*cfg)
		if err := reprocessar(*cfg, *nome); err != nil {
			log.Fatal(err)
		}

	default:
		uso()
	}
}
