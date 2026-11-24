package main

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// ---------------------------------------------------------------- auxiliares

// aguardar espera cond() valer ou falha o teste.
func aguardar(t *testing.T, oQue string, cond func() bool) {
	t.Helper()
	limite := time.Now().Add(3 * time.Second)
	for time.Now().Before(limite) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("tempo esgotado esperando: %s", oQue)
}

// soma de todas as classes
func concluidos(s *Server, desfecho string) float64 {
	t := 0.0
	for _, c := range classes {
		t += testutil.ToFloat64(s.concluidos.WithLabelValues(desfecho, c))
	}
	return t
}

func descartados(s *Server, motivo string) float64 {
	t := 0.0
	for _, c := range classes {
		t += testutil.ToFloat64(s.descartados.WithLabelValues(motivo, c))
	}
	return t
}

func tamanhoFila(s *Server) int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.fila) }

// servidor de teste cujo trabalho espera ser liberado
func servidorTravado(t *testing.T, workers, fila int, pol Politica) (*Server, *httptest.Server, chan struct{}) {
	libera := make(chan struct{})
	s := NewServer(Config{Workers: workers, Queue: fila, Politica: pol}, func(context.Context, uint64) error {
		<-libera
		return nil
	})
	s.Start()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts, libera
}

// pedir faz um GET /work com cabeçalhos opcionais e devolve o status.
func pedir(ts *httptest.Server, cliente *http.Client, orcamentoMs int, classe string) int {
	req, _ := http.NewRequest("GET", ts.URL+"/work", nil)
	if orcamentoMs > 0 {
		req.Header.Set("X-Budget-Ms", strconv.Itoa(orcamentoMs))
	}
	if classe != "" {
		req.Header.Set("X-Client-Class", classe)
	}
	if cliente == nil {
		cliente = http.DefaultClient
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return 0
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func pol(nome string) Politica { p := PoliticaPadrao(); p.Nome = nome; return p }

// servidor sem workers, para testar retirar() com tempo controlado
func servidorParado(p Politica) *Server {
	return NewServer(Config{Workers: 1, Queue: 64, Politica: p}, nil)
}

func jobCom(seq uint64, enq time.Time, prazo time.Time) *job {
	return &job{seq: seq, enqueued: enq, deadline: prazo, classe: ClasseOutra, clientCtx: context.Background(), done: make(chan error, 1)}
}

// ----------------------------------------------- lógica de cada política

func TestRetirarFila(t *testing.T) {
	s := servidorParado(pol(PolFixa))
	agora := time.Now()
	s.fila = []*job{jobCom(1, agora.Add(-5*time.Second), time.Time{}), jobCom(2, agora, time.Time{})}
	j, motivo, velhos := s.retirar(agora)
	if j.seq != 1 || motivo != "" || velhos != nil {
		t.Fatalf("fifo devia entregar o mais antigo sem descartar, por mais velho que seja: seq=%d motivo=%q", j.seq, motivo)
	}
}

func TestRetirarTempoFixo(t *testing.T) {
	s := servidorParado(pol(PolTempo)) // W = 100 ms
	agora := time.Now()
	s.fila = []*job{
		jobCom(1, agora.Add(-150*time.Millisecond), time.Time{}),
		jobCom(2, agora.Add(-60*time.Millisecond), time.Time{}),
	}
	if j, motivo, _ := s.retirar(agora); j.seq != 1 || motivo != MotivoExpirado {
		t.Fatalf("esperou 150 ms > W: devia expirar; seq=%d motivo=%q", j.seq, motivo)
	}
	if j, motivo, _ := s.retirar(agora); j.seq != 2 || motivo != "" {
		t.Fatalf("esperou 60 ms < W: devia ser processado; seq=%d motivo=%q", j.seq, motivo)
	}
}

func TestRetirarPrazo(t *testing.T) {
	s := servidorParado(pol(PolPrazo)) // Ŝ = 42 ms, margem 10 ms
	agora := time.Now()
	s.fila = []*job{
		jobCom(1, agora.Add(-60*time.Millisecond), agora.Add(40*time.Millisecond)),  // faltam 40 < 52: descarta
		jobCom(2, agora.Add(-60*time.Millisecond), agora.Add(140*time.Millisecond)), // faltam 140: processa
		jobCom(3, agora.Add(-5*time.Second), time.Time{}),                           // sem prazo: processa
	}
	for _, quer := range []struct {
		seq    uint64
		motivo string
	}{{1, MotivoPrazo}, {2, ""}, {3, ""}} {
		j, motivo, _ := s.retirar(agora)
		if j.seq != quer.seq || motivo != quer.motivo {
			t.Fatalf("seq=%d motivo=%q, esperava seq=%d motivo=%q", j.seq, motivo, quer.seq, quer.motivo)
		}
	}
	// A mesma espera de 60 ms, com prazos diferentes, dá decisões diferentes:
	// é isso que distingue a política 3 da 2.
}

func TestRetirarLIFOSemSobrecarga(t *testing.T) {
	s := servidorParado(pol(PolLIFO))
	agora := time.Now()
	s.ultimaVazia = agora.Add(-50 * time.Millisecond) // < janela de 100 ms
	s.fila = []*job{jobCom(1, agora.Add(-20*time.Millisecond), time.Time{}), jobCom(2, agora, time.Time{})}
	if j, motivo, _ := s.retirar(agora); j.seq != 1 || motivo != "" {
		t.Fatalf("sem sobrecarga a ordem é FIFO: seq=%d motivo=%q", j.seq, motivo)
	}
	if s.modoLIFO.Load() {
		t.Fatal("modo LIFO não devia estar ligado")
	}
}

func TestRetirarLIFOEmSobrecarga(t *testing.T) {
	s := servidorParado(pol(PolLIFO))
	agora := time.Now()
	s.ultimaVazia = agora.Add(-300 * time.Millisecond) // fila cheia há 300 ms > janela
	s.fila = []*job{
		jobCom(1, agora.Add(-300*time.Millisecond), time.Time{}), // expirado
		jobCom(2, agora.Add(-150*time.Millisecond), time.Time{}), // expirado
		jobCom(3, agora.Add(-50*time.Millisecond), time.Time{}),
		jobCom(4, agora.Add(-1*time.Millisecond), time.Time{}), // o mais novo
	}
	j, motivo, velhos := s.retirar(agora)
	if j.seq != 4 || motivo != "" {
		t.Fatalf("em sobrecarga devia entregar o mais novo: seq=%d motivo=%q", j.seq, motivo)
	}
	if len(velhos) != 2 || velhos[0].seq != 1 || velhos[1].seq != 2 {
		t.Fatalf("os dois expirados da ponta antiga deviam sair: %v", velhos)
	}
	if len(s.fila) != 1 || s.fila[0].seq != 3 {
		t.Fatalf("devia sobrar só o pedido 3 na fila")
	}
	if !s.modoLIFO.Load() || testutil.ToFloat64(s.trocasLIFO) != 1 {
		t.Fatal("modo LIFO devia estar ligado e contado uma vez")
	}
	// Continua em sobrecarga: não conta outra troca.
	s.fila = append(s.fila, jobCom(5, agora, time.Time{}))
	s.retirar(agora.Add(time.Millisecond))
	if testutil.ToFloat64(s.trocasLIFO) != 1 {
		t.Fatal("permanecer em LIFO não é uma troca nova")
	}
}

func TestRetirarLIFOExpiraOMaisNovo(t *testing.T) {
	s := servidorParado(pol(PolLIFO))
	agora := time.Now()
	s.ultimaVazia = agora.Add(-time.Second)
	s.fila = []*job{jobCom(1, agora.Add(-200*time.Millisecond), time.Time{})}
	if j, motivo, _ := s.retirar(agora); j.seq != 1 || motivo != MotivoExpirado {
		t.Fatalf("o último pedido, mesmo sendo o mais novo, expira se passou de W: motivo=%q", motivo)
	}
	if len(s.fila) != 0 {
		t.Fatal("fila devia ficar vazia")
	}
}

func TestDetectorVaziaVoltaAoFIFO(t *testing.T) {
	s := servidorParado(pol(PolLIFO))
	agora := time.Now()
	s.ultimaVazia = agora.Add(-time.Second)
	s.fila = []*job{jobCom(1, agora, time.Time{})}
	s.retirar(agora) // liga o LIFO e esvazia a fila: registra ultimaVazia = agora
	if !s.modoLIFO.Load() {
		t.Fatal("devia estar em LIFO")
	}
	s.fila = []*job{jobCom(2, agora.Add(10*time.Millisecond), time.Time{}), jobCom(3, agora.Add(20*time.Millisecond), time.Time{})}
	if j, _, _ := s.retirar(agora.Add(30 * time.Millisecond)); j.seq != 2 || s.modoLIFO.Load() {
		t.Fatal("a fila esvaziou há 30 ms: devia ter voltado ao FIFO")
	}
}

func TestDetectorCoDel(t *testing.T) {
	p := pol(PolLIFO)
	p.Detector = DetCoDel // janela 100 ms, alvo 5 ms
	s := servidorParado(p)
	t0 := time.Now()
	s.janelaIni = t0
	encher := func(agora time.Time, idadeMs int) {
		s.fila = []*job{jobCom(1, agora.Add(-time.Duration(idadeMs)*time.Millisecond), time.Time{}), jobCom(2, agora, time.Time{})}
	}
	// Janela 1: o mais antigo da fila sempre com 20 ms → fila que não esvazia.
	encher(t0.Add(10*time.Millisecond), 20)
	s.retirar(t0.Add(10 * time.Millisecond))
	if s.modoLIFO.Load() {
		t.Fatal("o veredito só sai quando a janela fecha")
	}
	encher(t0.Add(110*time.Millisecond), 20)
	if j, _, _ := s.retirar(t0.Add(110 * time.Millisecond)); !s.modoLIFO.Load() || j.seq != 2 {
		t.Fatal("menor idade na janela = 20 ms > 5 ms: devia ligar o LIFO e entregar o mais novo")
	}
	// Janela 2: a fila chega a esvaziar (idade mínima 0) → desliga na janela seguinte.
	s.fila = nil
	s.sobrecarregado(t0.Add(150 * time.Millisecond))
	encher(t0.Add(215*time.Millisecond), 2)
	s.retirar(t0.Add(215 * time.Millisecond))
	if s.modoLIFO.Load() {
		t.Fatal("a fila esvaziou na janela anterior: devia voltar ao FIFO")
	}
}

// ------------------------------------------------ comportamento pelo HTTP

// Políticas 1 a 3: fila cheia recusa na hora.
func TestFilaCheiaRejeitaCom503(t *testing.T) {
	for _, nome := range []string{PolFixa, PolTempo, PolPrazo} {
		t.Run(nome, func(t *testing.T) {
			s, ts, libera := servidorTravado(t, 1, 2, pol(nome))
			var wg sync.WaitGroup
			status := make(chan int, 3)
			for i := 0; i < 3; i++ { // 1 em serviço + 2 na fila
				wg.Add(1)
				go func() { defer wg.Done(); status <- pedir(ts, nil, 0, "") }()
				n := i + 1
				aguardar(t, "pedido entrar", func() bool { c, _ := s.sis.ler(); return c == n })
			}
			aguardar(t, "fila cheia", func() bool { return s.busy.Load() == 1 && tamanhoFila(s) == 2 })
			inicio := time.Now()
			if st := pedir(ts, nil, 0, ""); st != http.StatusServiceUnavailable {
				t.Fatalf("esperava 503 com fila cheia, veio %d", st)
			}
			if time.Since(inicio) > 500*time.Millisecond {
				t.Fatal("rejeição devia ser imediata")
			}
			if descartados(s, MotivoCheia) != 1 {
				t.Fatalf("full = %v, esperava 1", descartados(s, MotivoCheia))
			}
			close(libera)
			wg.Wait()
			close(status)
			for st := range status {
				if st != http.StatusOK {
					t.Fatalf("pedido aceito devia terminar com 200, veio %d", st)
				}
			}
			aguardar(t, "sistema esvaziar", func() bool { n, _ := s.sis.ler(); return n == 0 })
		})
	}
}

// LIFO: fila cheia aceita o novo e despeja o mais antigo.
func TestLIFODespejaOMaisAntigo(t *testing.T) {
	s, ts, libera := servidorTravado(t, 1, 2, pol(PolLIFO))
	status := make([]int, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ { // 1 em serviço, 2 na fila, o 4º despeja o 2º
		wg.Add(1)
		go func(i int) { defer wg.Done(); status[i] = pedir(ts, nil, 0, "") }(i)
		n := i + 1
		aguardar(t, "pedido entrar", func() bool { return testutil.ToFloat64(s.recebidos.WithLabelValues(ClasseOutra)) == float64(n) })
		aguardar(t, "pedido assentar", func() bool { c, _ := s.sis.ler(); return c == min(n, 3) })
	}
	aguardar(t, "despejo", func() bool { return descartados(s, MotivoDespejado) == 1 })
	if descartados(s, MotivoCheia) != 0 {
		t.Fatal("LIFO nunca recusa por fila cheia")
	}
	close(libera)
	wg.Wait()
	if status[1] != http.StatusServiceUnavailable {
		t.Fatalf("o despejado (2º pedido) devia receber 503, veio %d", status[1])
	}
	for _, i := range []int{0, 2, 3} {
		if status[i] != http.StatusOK {
			t.Fatalf("pedido %d devia terminar com 200, veio %d", i+1, status[i])
		}
	}
	aguardar(t, "sistema esvaziar", func() bool { n, _ := s.sis.ler(); return n == 0 })
}

// Sem descarte (política 1), o pedido cujo cliente desistiu é processado e
// conta como trabalho desperdiçado.
func TestFilaFixaDesperdica(t *testing.T) {
	s, ts, libera := servidorTravado(t, 1, 4, pol(PolFixa))
	cliente := &http.Client{Timeout: 50 * time.Millisecond}
	for i := 0; i < 3; i++ {
		if st := pedir(ts, cliente, 0, "patient"); st != 0 {
			t.Fatal("esperava timeout no cliente")
		}
	}
	aguardar(t, "três no sistema", func() bool { n, _ := s.sis.ler(); return n == 3 })
	time.Sleep(50 * time.Millisecond) // o servidor percebe as desconexões
	close(libera)
	aguardar(t, "três desperdiçados", func() bool { return concluidos(s, "wasted") == 3 })
	if testutil.ToFloat64(s.concluidos.WithLabelValues("wasted", "patient")) != 3 {
		t.Fatal("desperdício devia ser contado na classe patient")
	}
}

// Política 2: quem esperou mais que W recebe 503 sem ocupar worker; descarte
// de quem ainda tinha prazo conta como desnecessário.
func TestTempoFixoDescartaComTempoEDesnecessarios(t *testing.T) {
	p := pol(PolTempo)
	p.MaxEspera, p.SHat, p.Margem = 30*time.Millisecond, 5*time.Millisecond, 0
	s, ts, libera := servidorTravado(t, 1, 8, p)
	status := make(chan int, 3)
	go func() { status <- pedir(ts, nil, 5000, "patient") }() // ocupa o worker
	aguardar(t, "worker ocupado", func() bool { return s.busy.Load() == 1 })
	go func() { status <- pedir(ts, nil, 5000, "patient") }() // tem prazo de sobra: descarte desnecessário
	go func() { status <- pedir(ts, nil, 40, "impatient") }() // prazo vence: descarte necessário
	aguardar(t, "dois na fila", func() bool { return tamanhoFila(s) == 2 })
	time.Sleep(80 * time.Millisecond)
	close(libera)
	contagem := map[int]int{}
	for i := 0; i < 3; i++ {
		contagem[<-status]++
	}
	if contagem[200] != 1 || contagem[503] != 2 {
		t.Fatalf("esperava 1×200 e 2×503: %v", contagem)
	}
	if descartados(s, MotivoExpirado) != 2 {
		t.Fatalf("expired = %v", descartados(s, MotivoExpirado))
	}
	if testutil.ToFloat64(s.inuteis.WithLabelValues("patient")) != 1 || testutil.ToFloat64(s.inuteis.WithLabelValues("impatient")) != 0 {
		t.Fatal("só o descarte do paciente é desnecessário")
	}
}

// Política 3: a mesma espera dá decisões diferentes conforme o prazo.
func TestPrazoPorPedido(t *testing.T) {
	p := pol(PolPrazo)
	p.SHat, p.Margem = 10*time.Millisecond, 5*time.Millisecond
	s, ts, libera := servidorTravado(t, 1, 8, p)
	ocupa, curto, longo := make(chan int, 1), make(chan int, 1), make(chan int, 1)
	go func() { ocupa <- pedir(ts, nil, 0, "") }()
	aguardar(t, "worker ocupado", func() bool { return s.busy.Load() == 1 })
	go func() { curto <- pedir(ts, nil, 60, "impatient") }()
	aguardar(t, "curto na fila", func() bool { return tamanhoFila(s) == 1 })
	go func() { longo <- pedir(ts, nil, 2000, "patient") }()
	aguardar(t, "longo na fila", func() bool { return tamanhoFila(s) == 2 })
	time.Sleep(70 * time.Millisecond) // > 60 − 15
	close(libera)
	if <-ocupa != 200 || <-curto != 503 || <-longo != 200 {
		t.Fatal("o de prazo curto devia ser descartado e o de prazo longo processado")
	}
	if descartados(s, MotivoPrazo) != 1 || testutil.ToFloat64(s.inuteis.WithLabelValues("impatient")) != 0 {
		t.Fatal("um descarte por prazo, e não desnecessário")
	}
}

// A política muda com o serviço no ar pelo endpoint de administração.
func TestAdminTrocaPolitica(t *testing.T) {
	s, ts, _ := servidorTravado(t, 1, 4, pol(PolFixa))
	resp, err := http.Post(ts.URL+"/admin/policy?name=lifo&detector=codel&max_wait=80", "", nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("troca falhou: %v %v", err, resp)
	}
	resp.Body.Close()
	p := s.Politica()
	if p.Nome != PolLIFO || p.Detector != DetCoDel || p.MaxEspera != 80*time.Millisecond || p.SHat != 42*time.Millisecond {
		t.Fatalf("política = %+v", p)
	}
	resp, _ = http.Post(ts.URL+"/admin/policy?name=nada", "", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatal("política inválida devia dar 400")
	}
	resp.Body.Close()
	if s.Politica().Nome != PolLIFO {
		t.Fatal("pedido inválido não pode mudar a política")
	}
}

// Com C workers e trabalho de duração fixa, a vazão máxima é C/T, em todas as
// políticas, desde que ninguém expire.
func TestCapacidadeIgualCsobreT(t *testing.T) {
	for _, nome := range []string{PolFixa, PolTempo, PolPrazo, PolLIFO} {
		t.Run(nome, func(t *testing.T) {
			const c, tms, n = 4, 20, 80
			p := pol(nome)
			p.MaxEspera = 10 * time.Second
			s := NewServer(Config{Workers: c, Queue: n, Politica: p}, func(context.Context, uint64) error {
				time.Sleep(tms * time.Millisecond)
				return nil
			})
			s.Start()
			ts := httptest.NewServer(s.Handler())
			defer ts.Close()
			var wg sync.WaitGroup
			inicio := time.Now()
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if st := pedir(ts, nil, 0, ""); st != 200 {
						t.Errorf("status %d", st)
					}
				}()
			}
			wg.Wait()
			dur := time.Since(inicio).Seconds()
			esperado := float64(n) / (float64(c) * 1000 / tms)
			if dur < esperado*0.95 || dur > esperado*1.5 {
				t.Fatalf("%d pedidos levaram %.3fs; esperava ≈ %.3fs", n, dur, esperado)
			}
		})
	}
}

// ∫N(t)dt bate com a soma das permanências (lei de Little na forma exata).
func TestIntegradorBateComPermanencia(t *testing.T) {
	s := NewServer(Config{Workers: 2, Queue: 50}, func(context.Context, uint64) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	s.Start()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); pedir(ts, nil, 0, "") }()
	}
	wg.Wait()
	aguardar(t, "sistema esvaziar", func() bool { n, _ := s.sis.ler(); return n == 0 })
	_, area := s.sis.ler()
	corpo := metricas(t, ts.URL)
	soma := valorMetrica(t, corpo, "svc_sojourn_seconds_sum")
	if math.Abs(area-soma) > 0.02*soma+0.005 {
		t.Fatalf("∫N dt = %.4f, Σ permanência = %.4f", area, soma)
	}
}

func metricas(t *testing.T, base string) string {
	t.Helper()
	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func valorMetrica(t *testing.T, corpo, nome string) float64 {
	t.Helper()
	for _, linha := range strings.Split(corpo, "\n") {
		if strings.HasPrefix(linha, nome+" ") {
			v, err := strconv.ParseFloat(strings.TrimPrefix(linha, nome+" "), 64)
			if err != nil {
				t.Fatalf("valor inválido em %q", linha)
			}
			return v
		}
	}
	t.Fatalf("métrica %s não encontrada", nome)
	return 0
}

func TestMetricasExpostasDesdeOInicio(t *testing.T) {
	s := NewServer(Config{Workers: 3, Queue: 7, ServiceMs: 40}, func(context.Context, uint64) error { return nil })
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	corpo := metricas(t, ts.URL)
	for _, nome := range []string{
		`svc_requests_received_total{class="patient"} 0`,
		`svc_requests_completed_total{class="impatient",outcome="wasted"} 0`,
		`svc_requests_dropped_total{class="patient",reason="deadline"} 0`,
		`svc_requests_dropped_total{class="other",reason="evicted"} 0`,
		`svc_requests_needless_drops_total{class="impatient"} 0`,
		"svc_lifo_switches_total 0",
		"svc_lifo_active 0",
		"svc_queue_length 0",
		"svc_in_system_seconds_total",
		"svc_config_queue 7",
		"svc_sojourn_seconds_bucket",
	} {
		if !strings.Contains(corpo, nome) {
			t.Errorf("faltou em /metrics: %s", nome)
		}
	}
}
