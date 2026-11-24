package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// WorkFunc é o trabalho feito por um worker para um pedido. No serviço real é
// a consulta ao Postgres; nos testes é uma função controlada.
type WorkFunc func(ctx context.Context, seq uint64) error

// Políticas de fila (ver plano.md, seção 4).
const (
	PolFixa     = "fifo"      // 1: fila fixa, nunca descarta
	PolTempo    = "timebound" // 2: descarta na saída quem esperou mais que W
	PolPrazo    = "deadline"  // 3: descarta na saída quem não termina antes do próprio prazo
	PolLIFO     = "lifo"      // 4: LIFO quando sobrecarregado, expira com W
	DetVazia    = "empty"     // sobrecarga = fila não esvazia há mais que a janela
	DetCoDel    = "codel"     // sobrecarga = menor espera na janela acima do alvo
	ClasseOutra = "other"
)

var politicasValidas = map[string]bool{PolFixa: true, PolTempo: true, PolPrazo: true, PolLIFO: true}
var classes = []string{"impatient", "patient", ClasseOutra}

// Motivos de descarte.
const (
	MotivoCheia     = "full"     // chegou com a fila cheia (políticas 1 a 3)
	MotivoDespejado = "evicted"  // era o mais antigo quando chegou um novo com a fila cheia (LIFO)
	MotivoExpirado  = "expired"  // esperou mais que W (políticas 2 e 4)
	MotivoPrazo     = "deadline" // não terminaria antes do prazo do cliente (política 3)
	MotivoVelho     = "stale"    // expirado, removido da ponta antiga em sobrecarga (LIFO)
)

var motivos = []string{MotivoCheia, MotivoDespejado, MotivoExpirado, MotivoPrazo, MotivoVelho}

var errDescartado = errors.New("descartado pela política de fila")

// Politica é a parte da configuração que pode mudar com o serviço no ar.
type Politica struct {
	Nome       string        `json:"policy"`
	MaxEspera  time.Duration `json:"-"` // W
	SHat       time.Duration `json:"-"` // Ŝ: tempo de serviço esperado
	Margem     time.Duration `json:"-"`
	Janela     time.Duration `json:"-"` // janela do detector de sobrecarga
	Detector   string        `json:"detector"`
	AlvoCoDel  time.Duration `json:"-"`
	MaxEsperaM float64       `json:"max_wait_ms"`
	SHatM      float64       `json:"s_hat_ms"`
	MargemM    float64       `json:"margin_ms"`
	JanelaM    float64       `json:"overload_window_ms"`
	AlvoCoDelM float64       `json:"codel_target_ms"`
}

func (p *Politica) preencherMs() {
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	p.MaxEsperaM, p.SHatM, p.MargemM, p.JanelaM, p.AlvoCoDelM = ms(p.MaxEspera), ms(p.SHat), ms(p.Margem), ms(p.Janela), ms(p.AlvoCoDel)
}

// PoliticaPadrao são os parâmetros do plano.
func PoliticaPadrao() Politica {
	return Politica{
		Nome: PolFixa, MaxEspera: 100 * time.Millisecond, SHat: 42 * time.Millisecond,
		Margem: 10 * time.Millisecond, Janela: 100 * time.Millisecond, Detector: DetVazia,
		AlvoCoDel: 5 * time.Millisecond,
	}
}

// Config são os parâmetros fixos do serviço.
type Config struct {
	Workers   int // C: tamanho fixo do pool
	Queue     int // Q: posições de espera (além dos C pedidos em serviço)
	ServiceMs int // T: só informativo aqui; o tempo real vem da WorkFunc
	Politica  Politica
}

type job struct {
	seq       uint64
	enqueued  time.Time
	deadline  time.Time // zero se o cliente não mandou X-Budget-Ms
	classe    string
	clientCtx context.Context // só para saber se o cliente ainda espera
	done      chan error
}

// integrador acumula ∫N(t)dt, onde N(t) é o número de pedidos no sistema
// (em fila + em serviço). É a medida exata de L usada na lei de Little.
type integrador struct {
	mu     sync.Mutex
	n      int
	ultimo time.Time
	area   float64 // pedidos·segundo
}

func (i *integrador) somar(delta int) {
	i.mu.Lock()
	agora := time.Now()
	i.area += float64(i.n) * agora.Sub(i.ultimo).Seconds()
	i.n += delta
	i.ultimo = agora
	i.mu.Unlock()
}

func (i *integrador) ler() (n int, area float64) {
	i.mu.Lock()
	agora := time.Now()
	i.area += float64(i.n) * agora.Sub(i.ultimo).Seconds()
	i.ultimo = agora
	n, area = i.n, i.area
	i.mu.Unlock()
	return
}

// Server é o serviço: fila limitada + pool fixo, sem cancelamento do trabalho
// em andamento. A fila é um deque protegido por mutex: as políticas precisam
// olhar e tirar das duas pontas.
type Server struct {
	cfg  Config
	work WorkFunc
	seq  atomic.Uint64
	busy atomic.Int64
	sis  integrador

	mu   sync.Mutex
	cond *sync.Cond
	fila []*job // fila[0] é o mais antigo
	pol  Politica

	// Detector "empty": último instante em que a fila foi vista vazia.
	ultimaVazia time.Time
	// Detector "codel": menor espera vista na janela corrente.
	janelaIni   time.Time
	menorEspera time.Duration
	sobreCoDel  bool
	// Estado de sobrecarga calculado na última retirada (só para métricas).
	modoLIFO atomic.Bool

	reg         *prometheus.Registry
	recebidos   *prometheus.CounterVec
	concluidos  *prometheus.CounterVec
	descartados *prometheus.CounterVec
	inuteis     *prometheus.CounterVec
	trocasLIFO  prometheus.Counter
	permanenc   prometheus.Histogram
	espera      prometheus.Histogram
	servico     prometheus.Histogram
}

// Limites dos histogramas, em segundos.
var buckets = []float64{
	.005, .010, .020, .030, .040, .045, .050, .060, .075, .100, .125, .150,
	.200, .250, .300, .400, .500, .750, 1, 1.5, 2, 3, 5, 10,
}

func NewServer(cfg Config, work WorkFunc) *Server {
	if cfg.Politica.Nome == "" {
		cfg.Politica = PoliticaPadrao()
	}
	s := &Server{cfg: cfg, work: work, pol: cfg.Politica, reg: prometheus.NewRegistry()}
	s.cond = sync.NewCond(&s.mu)
	agora := time.Now()
	s.sis.ultimo, s.ultimaVazia, s.janelaIni = agora, agora, agora
	s.menorEspera = time.Duration(math.MaxInt64)

	s.recebidos = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "svc_requests_received_total", Help: "Tentativas recebidas em /work, por classe de cliente.",
	}, []string{"class"})
	s.concluidos = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "svc_requests_completed_total",
		Help: "Pedidos processados por um worker: useful (cliente ainda esperava), wasted (cliente já tinha desistido), error (falha no banco).",
	}, []string{"outcome", "class"})
	s.descartados = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "svc_requests_dropped_total", Help: "Pedidos descartados sem processar, por motivo e classe.",
	}, []string{"reason", "class"})
	s.inuteis = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "svc_requests_needless_drops_total",
		Help: "Descartes (exceto fila cheia) de pedidos que ainda terminariam no prazo: now + Ŝ + margem <= prazo.",
	}, []string{"class"})
	s.trocasLIFO = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "svc_lifo_switches_total", Help: "Vezes que o detector de sobrecarga ligou o modo LIFO.",
	})
	s.permanenc = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "svc_sojourn_seconds", Help: "Tempo no sistema (fila + serviço) dos pedidos processados.", Buckets: buckets,
	})
	s.espera = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "svc_queue_wait_seconds", Help: "Tempo de espera na fila dos pedidos processados.", Buckets: buckets,
	})
	s.servico = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "svc_service_seconds", Help: "Tempo de serviço (consulta ao banco).", Buckets: buckets,
	})
	s.reg.MustRegister(s.recebidos, s.concluidos, s.descartados, s.inuteis, s.trocasLIFO, s.permanenc, s.espera, s.servico)

	// Todas as séries existem desde o início, mesmo zeradas: a primeira
	// consulta ao Prometheus não pode voltar vazia.
	for _, c := range classes {
		s.recebidos.WithLabelValues(c)
		s.inuteis.WithLabelValues(c)
		for _, o := range []string{"useful", "wasted", "error"} {
			s.concluidos.WithLabelValues(o, c)
		}
		for _, m := range motivos {
			s.descartados.WithLabelValues(m, c)
		}
	}
	s.reg.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "svc_queue_length", Help: "Pedidos esperando na fila."},
			func() float64 { s.mu.Lock(); defer s.mu.Unlock(); return float64(len(s.fila)) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "svc_workers_busy", Help: "Workers ocupados."},
			func() float64 { return float64(s.busy.Load()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "svc_in_system", Help: "Pedidos no sistema (fila + serviço)."},
			func() float64 { n, _ := s.sis.ler(); return float64(n) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "svc_in_system_seconds_total", Help: "Integral no tempo de svc_in_system."},
			func() float64 { _, a := s.sis.ler(); return a }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "svc_lifo_active", Help: "1 se a política é LIFO e o detector indica sobrecarga."},
			func() float64 {
				if s.modoLIFO.Load() {
					return 1
				}
				return 0
			}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "svc_config_workers", Help: "C configurado."},
			func() float64 { return float64(cfg.Workers) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "svc_config_queue", Help: "Q configurado."},
			func() float64 { return float64(cfg.Queue) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "svc_config_service_ms", Help: "T configurado, em ms."},
			func() float64 { return float64(cfg.ServiceMs) }),
	)
	return s
}

// Start sobe os C workers.
func (s *Server) Start() {
	for i := 0; i < s.cfg.Workers; i++ {
		go s.worker()
	}
}

// DefinirPolitica troca a política com o serviço no ar.
func (s *Server) DefinirPolitica(p Politica) error {
	if !politicasValidas[p.Nome] {
		return fmt.Errorf("política inválida: %q", p.Nome)
	}
	if p.Detector != DetVazia && p.Detector != DetCoDel {
		return fmt.Errorf("detector inválido: %q", p.Detector)
	}
	s.mu.Lock()
	s.pol = p
	agora := time.Now()
	if len(s.fila) == 0 {
		s.ultimaVazia = agora
	}
	s.janelaIni, s.menorEspera, s.sobreCoDel = agora, time.Duration(math.MaxInt64), false
	s.mu.Unlock()
	s.modoLIFO.Store(false)
	return nil
}

// Politica devolve a política em vigor.
func (s *Server) Politica() Politica {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pol
}

// sobrecarregado aplica o detector escolhido. Chamado com s.mu travado.
func (s *Server) sobrecarregado(agora time.Time) bool {
	if s.pol.Detector == DetCoDel {
		// O veredito vale para a janela seguinte inteira; uma fila vazia
		// conta como espera zero.
		if len(s.fila) == 0 {
			s.menorEspera = 0
		}
		if agora.Sub(s.janelaIni) >= s.pol.Janela {
			s.sobreCoDel = s.menorEspera != time.Duration(math.MaxInt64) && s.menorEspera > s.pol.AlvoCoDel
			s.janelaIni, s.menorEspera = agora, time.Duration(math.MaxInt64)
		}
		return s.sobreCoDel
	}
	return agora.Sub(s.ultimaVazia) > s.pol.Janela
}

// notarEspera alimenta o detector CoDel.
func (s *Server) notarEspera(w time.Duration) {
	if w < s.menorEspera {
		s.menorEspera = w
	}
}

// retirar escolhe o próximo pedido segundo a política. Chamado com s.mu
// travado e a fila não vazia. Devolve o pedido, o motivo se ele deve ser
// descartado ("" se deve ser processado) e os pedidos velhos removidos da
// ponta antiga (LIFO).
func (s *Server) retirar(agora time.Time) (j *job, motivo string, velhos []*job) {
	frente := func() *job { x := s.fila[0]; s.fila[0] = nil; s.fila = s.fila[1:]; return x }
	tras := func() *job { n := len(s.fila); x := s.fila[n-1]; s.fila[n-1] = nil; s.fila = s.fila[:n-1]; return x }
	// O detector CoDel olha a idade do pedido MAIS ANTIGO da fila, e não a
	// espera de quem é servido: em modo LIFO quem é servido acabou de chegar,
	// e o detector desligaria o LIFO com a fila ainda cheia.
	s.notarEspera(agora.Sub(s.fila[0].enqueued))

	switch s.pol.Nome {
	case PolFixa:
		j = frente()
	case PolTempo:
		j = frente()
		if agora.Sub(j.enqueued) > s.pol.MaxEspera {
			motivo = MotivoExpirado
		}
	case PolPrazo:
		j = frente()
		if !j.deadline.IsZero() && agora.Add(s.pol.SHat+s.pol.Margem).After(j.deadline) {
			motivo = MotivoPrazo
		}
	case PolLIFO:
		sobre := s.sobrecarregado(agora)
		if sobre && !s.modoLIFO.Load() {
			s.trocasLIFO.Inc()
		}
		s.modoLIFO.Store(sobre)
		if sobre {
			// A fila está em ordem de chegada: os expirados são um prefixo.
			for len(s.fila) > 1 && agora.Sub(s.fila[0].enqueued) > s.pol.MaxEspera {
				velhos = append(velhos, frente())
			}
			j = tras()
		} else {
			j = frente()
		}
		if agora.Sub(j.enqueued) > s.pol.MaxEspera {
			motivo = MotivoExpirado
		}
	}
	if len(s.fila) == 0 {
		s.ultimaVazia = agora
	}
	return j, motivo, velhos
}

// descartar responde 503 a um pedido que não será processado.
func (s *Server) descartar(j *job, motivo string, agora time.Time) {
	s.sis.somar(-1)
	s.descartados.WithLabelValues(motivo, j.classe).Inc()
	pol := s.Politica()
	if motivo != MotivoCheia && !j.deadline.IsZero() && !agora.Add(pol.SHat+pol.Margem).After(j.deadline) {
		s.inuteis.WithLabelValues(j.classe).Inc()
	}
	j.done <- errDescartado
}

func (s *Server) worker() {
	for {
		s.mu.Lock()
		for len(s.fila) == 0 {
			s.ultimaVazia = time.Now()
			s.cond.Wait()
		}
		agora := time.Now()
		j, motivo, velhos := s.retirar(agora)
		s.mu.Unlock()

		for _, v := range velhos {
			s.descartar(v, MotivoVelho, agora)
		}
		if motivo != "" {
			s.descartar(j, motivo, agora)
			continue
		}
		s.processar(j, agora)
	}
}

func (s *Server) processar(j *job, inicio time.Time) {
	s.busy.Add(1)
	// context.Background(): o trabalho NÃO é cancelado quando o cliente
	// desiste. O limite de 60 s só protege contra um banco travado.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	err := s.work(ctx, j.seq)
	cancel()
	fim := time.Now()
	s.busy.Add(-1)
	s.sis.somar(-1)

	s.espera.Observe(inicio.Sub(j.enqueued).Seconds())
	s.servico.Observe(fim.Sub(inicio).Seconds())
	s.permanenc.Observe(fim.Sub(j.enqueued).Seconds())
	switch {
	case err != nil:
		s.concluidos.WithLabelValues("error", j.classe).Inc()
	case j.clientCtx.Err() != nil:
		s.concluidos.WithLabelValues("wasted", j.classe).Inc()
	default:
		s.concluidos.WithLabelValues("useful", j.classe).Inc()
	}
	j.done <- err
}

func classeDe(r *http.Request) string {
	switch c := r.Header.Get("X-Client-Class"); c {
	case "impatient", "patient":
		return c
	}
	return ClasseOutra
}

func (s *Server) handleWork(w http.ResponseWriter, r *http.Request) {
	agora := time.Now()
	j := &job{
		seq: s.seq.Add(1), enqueued: agora, classe: classeDe(r),
		clientCtx: r.Context(), done: make(chan error, 1),
	}
	if b, err := strconv.Atoi(r.Header.Get("X-Budget-Ms")); err == nil && b > 0 {
		j.deadline = agora.Add(time.Duration(b) * time.Millisecond)
	}
	s.recebidos.WithLabelValues(j.classe).Inc()
	// A contagem entra antes da fila: se o worker pegasse o pedido antes,
	// N(t) poderia ficar negativo por um instante.
	s.sis.somar(1)

	var despejado *job
	s.mu.Lock()
	if len(s.fila) == 0 {
		s.ultimaVazia = agora
	}
	if len(s.fila) >= s.cfg.Queue {
		if s.pol.Nome != PolLIFO {
			s.mu.Unlock()
			s.descartar(j, MotivoCheia, agora)
			<-j.done
			http.Error(w, "fila cheia", http.StatusServiceUnavailable)
			return
		}
		// LIFO: entra o novo, sai o mais antigo.
		despejado = s.fila[0]
		s.fila[0] = nil
		s.fila = s.fila[1:]
	}
	s.fila = append(s.fila, j)
	s.cond.Signal()
	s.mu.Unlock()
	if despejado != nil {
		s.descartar(despejado, MotivoDespejado, agora)
	}

	select {
	case err := <-j.done:
		switch {
		case errors.Is(err, errDescartado):
			http.Error(w, "descartado", http.StatusServiceUnavailable)
		case err != nil:
			http.Error(w, "erro no banco", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok\n"))
		}
	case <-r.Context().Done():
		// O cliente desistiu. O pedido continua na fila; a política decide.
	}
}

func (s *Server) estado() map[string]any {
	n, area := s.sis.ler()
	s.mu.Lock()
	tam := len(s.fila)
	p := s.pol
	s.mu.Unlock()
	p.preencherMs()
	return map[string]any{
		"workers": s.cfg.Workers, "queue_capacity": s.cfg.Queue, "service_ms": s.cfg.ServiceMs,
		"policy": p, "queue_length": tam, "workers_busy": s.busy.Load(),
		"lifo_active": s.modoLIFO.Load(), "in_system": n, "in_system_seconds": area,
	}
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.estado())
}

// handleAdmin troca a política: POST /admin/policy?name=lifo&detector=codel.
// Parâmetros ausentes mantêm o valor atual (em ms: max_wait, s_hat, margin,
// window, codel_target).
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.estado())
		return
	}
	p := s.Politica()
	q := r.URL.Query()
	if v := q.Get("name"); v != "" {
		p.Nome = v
	}
	if v := q.Get("detector"); v != "" {
		p.Detector = v
	}
	for nome, dst := range map[string]*time.Duration{
		"max_wait": &p.MaxEspera, "s_hat": &p.SHat, "margin": &p.Margem, "window": &p.Janela, "codel_target": &p.AlvoCoDel,
	} {
		if v := q.Get(nome); v != "" {
			ms, err := strconv.ParseFloat(v, 64)
			if err != nil || ms < 0 {
				http.Error(w, "valor inválido para "+nome, http.StatusBadRequest)
				return
			}
			*dst = time.Duration(ms * float64(time.Millisecond))
		}
	}
	if err := s.DefinirPolitica(p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.estado())
}

// Handler devolve as rotas do serviço.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/work", s.handleWork)
	mux.HandleFunc("/state", s.handleState)
	mux.HandleFunc("/admin/policy", s.handleAdmin)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.Handle("/metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))
	return mux
}
