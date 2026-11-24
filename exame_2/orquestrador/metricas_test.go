package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var nan = math.NaN()

func quase(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestFasesLinhas(t *testing.T) {
	f := Fases{Aquecimento: 15, Regime: 45, Atraso: 20, Recuperacao: 100}
	if f.Total() != 180 {
		t.Fatalf("total = %d", f.Total())
	}
	casos := map[string]Intervalo{
		FaseAquecimento: {1, 16},
		FaseRegime:      {16, 61},
		FaseAtraso:      {61, 81},
		FaseRecuperacao: {81, 181},
	}
	for fase, quer := range casos {
		if got := f.Linhas(fase); got != quer {
			t.Errorf("%s: %v, esperava %v", fase, got, quer)
		}
	}
	// Cada linha de 1 a Total pertence a exatamente uma fase, sem buracos.
	contagem := map[string]int{}
	for k := 1; k <= f.Total(); k++ {
		fase := f.FaseDaLinha(k)
		if fase == "" {
			t.Fatalf("linha %d sem fase", k)
		}
		contagem[fase]++
	}
	if contagem[FaseAquecimento] != 15 || contagem[FaseRegime] != 45 || contagem[FaseAtraso] != 20 || contagem[FaseRecuperacao] != 100 {
		t.Fatalf("contagem por fase errada: %v", contagem)
	}
	if f.FaseDaLinha(0) != "" || f.FaseDaLinha(181) != "" {
		t.Fatal("linhas fora da execução não têm fase")
	}
}

func TestDeltas(t *testing.T) {
	// Contador que ainda não existe (NaN), aparece, tem um buraco e segue.
	got := deltas([]float64{nan, nan, 10, 25, nan, 40})
	quer := []float64{0, 0, 10, 15, 0, 15}
	for i := range quer {
		if got[i] != quer[i] {
			t.Fatalf("deltas = %v, esperava %v", got, quer)
		}
	}
	// A linha 0 é só linha de base, mesmo que o contador já esteja alto.
	got = deltas([]float64{1000, 1100, 1250})
	if got[0] != 0 || got[1] != 100 || got[2] != 150 {
		t.Fatalf("deltas com linha de base = %v", got)
	}
	// Reinício do processo: o contador volta ao zero e recomeça.
	got = deltas([]float64{500, 520, 7, 30})
	if got[2] != 7 || got[3] != 23 {
		t.Fatalf("deltas com reinício = %v", got)
	}
}

func TestInterpolar(t *testing.T) {
	// Contador do serviço: amostras a cada 1 s, 0,3 s depois de cada instante
	// da grade, subindo 100 por segundo.
	var a []amostraBruta
	for i := -2; i <= 6; i++ {
		a = append(a, amostraBruta{1000.3 + float64(i), 100 * float64(i+2)})
	}
	got := interpolar(a, 1000, 5, false)
	for i, quer := range []float64{170, 270, 370, 470, 570} {
		if !quase(got[i], quer, 1e-6) {
			t.Fatalf("interpolado = %v, esperava começar em 170 e subir 100 por segundo", got)
		}
	}

	// O caso que motivou a função: amostras oscilando em torno da virada do
	// segundo. Com "última amostra antes do instante" os incrementos seriam
	// 0, 200, 0, 200; interpolando, ficam perto de 100.
	a = []amostraBruta{{999.98, 0}, {1001.02, 100}, {1001.98, 200}, {1003.02, 300}, {1003.98, 400}, {1005.02, 500}}
	d := deltas(interpolar(a, 1000, 6, false))
	for k := 1; k < 6; k++ {
		if d[k] < 90 || d[k] > 110 {
			t.Fatalf("incrementos = %v; deviam ficar perto de 100", d)
		}
	}

	// Contador do k6: nasce com a execução. Primeira amostra em t0+0,6 s com
	// 60 pedidos; antes disso sobe a partir de zero.
	a = []amostraBruta{{1000.6, 60}, {1001.6, 160}, {1002.6, 260}}
	got = interpolar(a, 1000, 4, true)
	if !quase(got[0], 24, 1e-6) || !quase(got[1], 100, 1e-6) || !quase(got[2], 200, 1e-6) {
		t.Fatalf("contador do k6 = %v", got)
	}
	if got[3] != 260 { // depois da última amostra fica constante
		t.Fatalf("depois da última amostra = %v", got[3])
	}
	// Sem zeroAntes, antes da primeira amostra vale o da primeira.
	if got = interpolar(a, 1000, 2, false); got[0] != 60 {
		t.Fatalf("antes da primeira amostra = %v", got[0])
	}

	// A soma dos incrementos é o total, qualquer que seja o alinhamento.
	a = []amostraBruta{{999.5, 0}, {1000.7, 10}, {1001.4, 35}, {1002.9, 50}, {1003.5, 80}}
	d = deltas(interpolar(a, 999.5, 6, false))
	if total := soma(d, Intervalo{1, 6}); !quase(total, 80, 1e-9) {
		t.Fatalf("soma dos incrementos = %v, esperava 80", total)
	}

	// Lacuna: o contador ficou parado em 100 por 20 s (sem amostras) e depois
	// voltou a subir. O incremento pertence ao último segundo, não aos 20.
	a = []amostraBruta{{1000, 100}, {1020.5, 150}, {1021.5, 250}}
	got = interpolar(a, 1000, 23, false)
	for i := 0; i <= 19; i++ {
		if got[i] != 100 {
			t.Fatalf("durante a lacuna o contador devia ficar em 100: %v", got)
		}
	}
	if !quase(got[20], 125, 1e-6) || !quase(got[21], 200, 1e-6) || got[22] != 250 {
		t.Fatalf("depois da lacuna = %v", got[20:])
	}

	// Sem amostras: tudo NaN (a métrica não existe).
	if got = interpolar(nil, 1000, 3, true); !math.IsNaN(got[0]) || !math.IsNaN(got[2]) {
		t.Fatalf("sem amostras = %v", got)
	}

	// Reinício do contador entre duas amostras: não interpola para baixo.
	a = []amostraBruta{{999.5, 500}, {1000.5, 20}, {1001.5, 120}}
	got = interpolar(a, 1000, 2, false)
	if got[0] != 500 || !quase(got[1], 70, 1e-6) {
		t.Fatalf("reinício = %v", got)
	}
}

func TestFmtNum(t *testing.T) {
	casos := map[float64]string{0: "0", 115: "115", 67.74521877: "67.75", 0.004: "0", -0.001: "0", 12.5: "12.5", 100.10: "100.1"}
	for v, quer := range casos {
		if got := fmtNum(v); got != quer {
			t.Errorf("fmtNum(%v) = %q, esperava %q", v, got, quer)
		}
	}
	if fmtNum(nan) != "" || fmtNum(math.Inf(1)) != "" {
		t.Error("NaN e Inf viram célula vazia")
	}
}

func TestSomarSeries(t *testing.T) {
	got := somarSeries([][]float64{{1, nan, 3}, {10, nan, nan}}, 3)
	if got[0] != 11 || !math.IsNaN(got[1]) || got[2] != 3 {
		t.Fatalf("soma = %v", got)
	}
	if got = somarSeries(nil, 2); !math.IsNaN(got[0]) {
		t.Fatalf("sem séries = %v", got)
	}
}

func TestMediaESoma(t *testing.T) {
	v := []float64{99, 1, 2, nan, 4}
	if got := soma(v, Intervalo{1, 5}); got != 7 {
		t.Fatalf("soma = %v", got)
	}
	if got := media(v, Intervalo{1, 5}); !quase(got, 7.0/3, 1e-12) {
		t.Fatalf("média ignora NaN: %v", got)
	}
	if got := media(v, Intervalo{3, 4}); !math.IsNaN(got) {
		t.Fatalf("média sem dados devia ser NaN: %v", got)
	}
	if got := soma(v, Intervalo{3, 99}); got != 4 {
		t.Fatalf("soma além do fim = %v", got)
	}
}

// séries de uma execução com 100 pedidos novos por segundo: goodput zero
// durante a falta (linhas 1..fimFalta) e depois os valores dados.
func seriesRecuperacao(fimFalta int, depois ...float64) (sucesso, originados []float64) {
	sucesso = append(make([]float64, fimFalta+1), depois...)
	originados = make([]float64, len(sucesso))
	for i := range originados {
		originados[i] = 100
	}
	return
}

func TestTempoRecuperacao(t *testing.T) {
	const regime, limiar, janela = 1.0, 0.95, 5

	// Volta na hora: os 5 s seguintes ao fim da falta já estão no regime.
	g, o := seriesRecuperacao(10, 100, 99, 101, 100, 98, 100, 100)
	if d, ok := tempoRecuperacao(g, o, 10, regime, limiar, janela); !ok || d != 0 {
		t.Fatalf("recuperação imediata: d=%d ok=%v", d, ok)
	}

	// Três segundos ruins antes de voltar.
	g, o = seriesRecuperacao(10, 0, 10, 40, 100, 100, 100, 100, 100, 100, 100)
	if d, ok := tempoRecuperacao(g, o, 10, regime, limiar, janela); !ok || d != 3 {
		t.Fatalf("esperava 3 s, veio d=%d ok=%v", d, ok)
	}

	// Preso: goodput zero até o fim.
	g, o = seriesRecuperacao(10, make([]float64, 60)...)
	if d, ok := tempoRecuperacao(g, o, 10, regime, limiar, janela); ok || d != -1 {
		t.Fatalf("metaestável não pode contar como recuperado: d=%d ok=%v", d, ok)
	}

	// 94 % do regime não basta; 95 % basta.
	g, o = seriesRecuperacao(10, 94, 94, 94, 94, 94, 94, 94)
	if _, ok := tempoRecuperacao(g, o, 10, regime, limiar, janela); ok {
		t.Fatal("94 % do regime não é recuperação")
	}
	g, o = seriesRecuperacao(10, 95, 95, 95, 95, 95, 95, 95)
	if d, ok := tempoRecuperacao(g, o, 10, regime, limiar, janela); !ok || d != 0 {
		t.Fatalf("95 %% do regime é recuperação: d=%d ok=%v", d, ok)
	}

	// Um pico isolado de um segundo não engana a janela de 5 s.
	g, o = seriesRecuperacao(10, 0, 300, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if _, ok := tempoRecuperacao(g, o, 10, regime, limiar, janela); ok {
		t.Fatal("pico isolado contou como recuperação")
	}

	// Os segundos da falta não entram na conta (a janela olha para a frente).
	g, o = seriesRecuperacao(10, 100, 100, 100, 100, 100, 100)
	if d, _ := tempoRecuperacao(g, o, 10, regime, limiar, janela); d != 0 {
		t.Fatalf("a falta contaminou a janela: d=%d", d)
	}

	// O critério é a fração atendida, não a contagem: uma rajada ou um vale
	// nas chegadas não muda o veredito se todos os pedidos são atendidos.
	g, o = seriesRecuperacao(10, 60, 70, 65, 150, 140, 100, 100)
	copy(o[11:], []float64{60, 70, 65, 150, 140, 100, 100})
	if d, ok := tempoRecuperacao(g, o, 10, regime, limiar, janela); !ok || d != 0 {
		t.Fatalf("chegadas irregulares, tudo atendido: d=%d ok=%v", d, ok)
	}

	// O alvo é relativo ao regime: se no regime 98 % eram atendidos, 94 % passa.
	g, o = seriesRecuperacao(10, 94, 94, 94, 94, 94, 94, 94)
	if d, ok := tempoRecuperacao(g, o, 10, 0.98, limiar, janela); !ok || d != 0 {
		t.Fatalf("alvo relativo ao regime: d=%d ok=%v", d, ok)
	}

	// Sem regime definido não há o que comparar.
	if _, ok := tempoRecuperacao(g, o, 10, 0, limiar, janela); ok {
		t.Fatal("regime zero não pode dar recuperação")
	}
	if _, ok := tempoRecuperacao(g, o, 10, nan, limiar, janela); ok {
		t.Fatal("regime NaN não pode dar recuperação")
	}
}

func TestFracaoAtendida(t *testing.T) {
	s := []float64{0, 90, 100, 110}
	o := []float64{0, 100, 100, 100}
	if got := fracaoAtendida(s, o, Intervalo{1, 4}); !quase(got, 1, 1e-12) {
		t.Fatalf("fração = %v, esperava 1", got)
	}
	if got := fracaoAtendida(s, o, Intervalo{1, 2}); !quase(got, 0.9, 1e-12) {
		t.Fatalf("fração = %v, esperava 0,9", got)
	}
	if got := fracaoAtendida(s, o, Intervalo{0, 1}); !math.IsNaN(got) {
		t.Fatalf("sem pedidos novos a fração é indefinida: %v", got)
	}
}

func TestQuantil(t *testing.T) {
	// 100 observações: 50 até 10 ms, 40 entre 10 e 20 ms, 10 entre 20 e 40 ms.
	h := Histograma{
		Limites:   []float64{0.010, 0.020, 0.040, math.Inf(1)},
		Contagens: []float64{50, 90, 100, 100},
	}
	if got := h.quantil(0.50); !quase(got, 0.010, 1e-9) {
		t.Errorf("p50 = %v, esperava 0,010", got)
	}
	// p75: 25 das 40 observações do segundo bucket → 10 + 10·25/40 = 16,25 ms.
	if got := h.quantil(0.75); !quase(got, 0.01625, 1e-9) {
		t.Errorf("p75 = %v, esperava 0,01625", got)
	}
	// p99: 9 das 10 observações do terceiro bucket → 20 + 20·0,9 = 38 ms.
	if got := h.quantil(0.99); !quase(got, 0.038, 1e-9) {
		t.Errorf("p99 = %v, esperava 0,038", got)
	}

	// Observações além do último limite finito: devolve esse limite.
	h = Histograma{Limites: []float64{1, 2, math.Inf(1)}, Contagens: []float64{0, 0, 10}}
	if got := h.quantil(0.99); got != 2 {
		t.Errorf("bucket aberto: %v, esperava 2", got)
	}

	// Sem observações.
	h = Histograma{Limites: []float64{1, math.Inf(1)}, Contagens: []float64{0, 0}}
	if got := h.quantil(0.99); !math.IsNaN(got) {
		t.Errorf("histograma vazio devia dar NaN: %v", got)
	}
}

func TestHistogramaNoIntervalo(t *testing.T) {
	limites := []float64{0.1, math.Inf(1)}
	porLimite := [][]float64{
		{0, 5, 5, 0, 0},   // ≤ 100 ms
		{0, 5, 5, 10, 10}, // total
	}
	h := histogramaNoIntervalo(limites, porLimite, Intervalo{1, 3})
	if h.Contagens[0] != 10 || h.Contagens[1] != 10 {
		t.Fatalf("contagens = %v", h.Contagens)
	}
	h = histogramaNoIntervalo(limites, porLimite, Intervalo{3, 5})
	if h.Contagens[0] != 0 || h.Contagens[1] != 20 {
		t.Fatalf("contagens = %v", h.Contagens)
	}
}

func TestClassificar(t *testing.T) {
	casos := []struct {
		regime, final float64
		quer          string
	}{
		{1.00, 1.00, DesfechoRecuperou},
		{0.99, 0.95, DesfechoRecuperou},
		{1.00, 0.00, DesfechoMetaestavel},
		{1.00, 0.50, DesfechoMetaestavel},
		{1.00, 0.70, DesfechoParcial},
		{0.80, 1.00, DesfechoSemRegime}, // já estava doente antes do gatilho
		{nan, 1.00, DesfechoSemRegime},
		{1.00, nan, DesfechoSemRegime},
	}
	for _, c := range casos {
		if got := classificar(c.regime, c.final); got != c.quer {
			t.Errorf("classificar(%v, %v) = %s, esperava %s", c.regime, c.final, got, c.quer)
		}
	}
}

func TestDesvioMaxPct(t *testing.T) {
	m, d := desvioMaxPct([]float64{190, 200, 210})
	if m != 200 || !quase(d, 5, 1e-9) {
		t.Fatalf("média %v, desvio %v", m, d)
	}
	m, d = desvioMaxPct([]float64{195, 195, 195})
	if m != 195 || d != 0 {
		t.Fatalf("média %v, desvio %v", m, d)
	}
}

func TestPlanoVarredura(t *testing.T) {
	p := PlanoVarredura{
		Filas: []string{"fifo", "timebound", "deadline", "lifo"}, Politicas: []string{"fixed", "backoff"},
		Retries: []int{3}, Cargas: []float64{20, 50, 70, 90}, Reps: 3, Semente: 27,
	}
	cs := p.cenarios(200)
	if len(cs) != 96 {
		t.Fatalf("o plano tem 96 execuções, veio %d", len(cs))
	}
	vistos := map[string]bool{}
	sementes := map[[2]float64]map[int]bool{}
	for i, c := range cs {
		if vistos[c.ID] {
			t.Fatalf("id repetido: %s", c.ID)
		}
		vistos[c.ID] = true
		// As repetições rodam em blocos de 32.
		if quer := i/32 + 1; c.Rep != quer {
			t.Fatalf("execução %d (%s) está na rep %d, esperava %d", i, c.ID, c.Rep, quer)
		}
		if !quase(c.Taxa, 200*c.CargaPct/100, 1e-9) {
			t.Fatalf("%s: taxa %v para carga %v %%", c.ID, c.Taxa, c.CargaPct)
		}
		k := [2]float64{c.CargaPct, float64(c.Rep)}
		if sementes[k] == nil {
			sementes[k] = map[int]bool{}
		}
		sementes[k][c.Seed] = true
	}
	// Números comuns: as 8 configurações de uma (carga, rep) usam a mesma
	// semente, e células diferentes usam sementes diferentes.
	if len(sementes) != 12 {
		t.Fatalf("esperava 12 células, vieram %d", len(sementes))
	}
	todas := map[int]bool{}
	for k, ss := range sementes {
		if len(ss) != 1 {
			t.Fatalf("célula %v com %d sementes; devia ter uma só", k, len(ss))
		}
		for sd := range ss {
			if todas[sd] {
				t.Fatalf("semente %d repetida entre células", sd)
			}
			todas[sd] = true
		}
	}
	for _, id := range []string{"fifo-fixed-r3-c20-rep1", "lifo-backoff-r3-c90-rep3", "deadline-fixed-r3-c70-rep2"} {
		if !vistos[id] {
			t.Fatalf("faltou o id %s", id)
		}
	}
	deNovo := p.cenarios(200)
	for i := range cs {
		if cs[i].ID != deNovo[i].ID {
			t.Fatal("a ordem mudou entre duas chamadas com a mesma semente")
		}
	}
	emOrdem := true
	for i := 1; i < 32; i++ {
		if cs[i].ID < cs[i-1].ID {
			emOrdem = false
		}
	}
	if emOrdem {
		t.Fatal("o bloco não foi embaralhado")
	}
}

func TestDerivarParametros(t *testing.T) {
	if got := timeoutMedio("impatient:100:0.5,patient:200:0.5"); got != 150 {
		t.Fatalf("timeout médio = %v", got)
	}
	cfg := Config{Classes: "impatient:100:0.5,patient:200:0.5", MargemMs: 10}
	derivarParametros(&cfg, Calibracao{Medidas: []MedidaMu{{TempoServicoMs: 41.6}, {TempoServicoMs: 42.4}}})
	if cfg.SHatMs != 42 || cfg.MaxWaitMs != 98 {
		t.Fatalf("Ŝ = %v, W = %v; esperava 42 e 98", cfg.SHatMs, cfg.MaxWaitMs)
	}
	// Valores fixados por opção não são sobrescritos.
	cfg = Config{Classes: "a:150:1", MargemMs: 10, SHatMs: 40, MaxWaitMs: 100}
	derivarParametros(&cfg, Calibracao{Medidas: []MedidaMu{{TempoServicoMs: 52}}})
	if cfg.SHatMs != 40 || cfg.MaxWaitMs != 100 {
		t.Fatalf("opções sobrescritas: %+v", cfg)
	}
}

func TestAvaliarSHat(t *testing.T) {
	cal := Calibracao{Medidas: []MedidaMu{
		{Mu: 190, TempoServicoMs: 42, ServicoN: 5700, MuLittle: 8 * 1000 / 42.0, ServicoP95Ms: 45},
		{Mu: 191, TempoServicoMs: 41.8, ServicoN: 5730, MuLittle: 8 * 1000 / 41.8, ServicoP95Ms: 44},
	}}
	avaliarSHat(&cal)
	if !quase(cal.SHatMs, (42*5700+41.8*5730)/11430, 1e-9) || !cal.SHatAprovado || cal.SHatP95Ms != 44.5 {
		t.Fatalf("Ŝ = %v, aprovado = %v, p95 = %v", cal.SHatMs, cal.SHatAprovado, cal.SHatP95Ms)
	}
	// O tempo medido não explica a vazão: reprovado.
	cal = Calibracao{Medidas: []MedidaMu{{Mu: 150, TempoServicoMs: 42, ServicoN: 100, MuLittle: 190}}}
	avaliarSHat(&cal)
	if cal.SHatAprovado {
		t.Fatal("μ contado 150 contra 190 pela lei de Little devia reprovar")
	}
	// derivarParametros usa o Ŝ da calibração, com uma casa decimal.
	cfg := Config{Classes: "a:150:1", MargemMs: 10}
	derivarParametros(&cfg, Calibracao{SHatMs: 41.87})
	if cfg.SHatMs != 41.9 || cfg.MaxWaitMs != 98 {
		t.Fatalf("Ŝ = %v, W = %v", cfg.SHatMs, cfg.MaxWaitMs)
	}
}

func TestHistogramaEntre(t *testing.T) {
	a := Amostra{Valores: map[string]float64{`h_bucket{le="0.04"}`: 10, `h_bucket{le="0.05"}`: 20, `h_bucket{le="+Inf"}`: 20}}
	b := Amostra{Valores: map[string]float64{`h_bucket{le="0.04"}`: 10, `h_bucket{le="0.05"}`: 120, `h_bucket{le="+Inf"}`: 120}}
	h := histogramaEntre(a, b, "h_bucket")
	if len(h.Limites) != 3 || h.Contagens[0] != 0 || h.Contagens[1] != 100 {
		t.Fatalf("histograma = %+v", h)
	}
	if got := h.quantil(0.5); !quase(got, 0.045, 1e-9) {
		t.Fatalf("p50 = %v", got)
	}
}

func TestMaiorTimeout(t *testing.T) {
	if got := maiorTimeout("impatient:100:0.5,patient:200:0.5"); got != 200 {
		t.Fatalf("maior timeout = %d", got)
	}
	if got := maiorTimeout("calibracao:5000:1"); got != 5000 {
		t.Fatalf("maior timeout = %d", got)
	}
}

func TestVusNecessarios(t *testing.T) {
	cfg := Config{JitterChegadaMs: 1000, BackoffBaseMs: 100, BackoffTetoMs: 1000}
	// Sem retry: 1 s de jitter + 150 ms → 1,15 s × 100/s × 1,3 + 50.
	pre, max := vusNecessarios(cfg, Cenario{Politica: "none", Taxa: 100}, 150)
	if pre != 200 || max != 400 {
		t.Fatalf("none: pre=%d max=%d", pre, max)
	}
	// Backoff com 3 retries: 1 + 4·0,15 + (0,1+0,2+0,4) = 2,3 s.
	pre, _ = vusNecessarios(cfg, Cenario{Politica: "backoff", MaxRetries: 3, Taxa: 100}, 150)
	if pre < 349 || pre > 350 {
		t.Fatalf("backoff: pre=%d, esperava 349", pre)
	}
	im, _ := vusNecessarios(cfg, Cenario{Politica: "immediate", MaxRetries: 3, Taxa: 100}, 150)
	if im >= pre || im <= 200 {
		t.Fatalf("immediate devia ficar entre none e backoff: %d", im)
	}
	cfg.FixedDelayMs = 50
	fx, _ := vusNecessarios(cfg, Cenario{Politica: "fixed", MaxRetries: 3, Taxa: 100}, 150)
	if fx != im+20 { // 3 × 50 ms a mais por iteração × 100/s × 1,3 ≈ 20
		t.Fatalf("fixed: pre=%d, esperava %d", fx, im+20)
	}
}

func TestResumir(t *testing.T) {
	cfg := Config{
		Fases:            Fases{Aquecimento: 2, Regime: 4, Atraso: 2, Recuperacao: 12},
		LimiarRecuperado: 0.95, JanelaRecupS: 3, JanelaFinalS: 5,
	}
	n := cfg.Fases.Total() + 1
	rep := func(v float64) []float64 {
		s := make([]float64, n)
		for i := 1; i < n; i++ {
			s[i] = v
		}
		return s
	}
	zeros := func() []float64 { return make([]float64, n) }
	s := Series{
		N: n, Originados: rep(100), Tentativas: rep(100), Sucesso: rep(100), LatOkMs: rep(100 * 50),
		Recebidos: rep(100), Area: rep(5), Fila: rep(1), Uteis: rep(100),
		Timeouts: zeros(), RejCliente: zeros(), Falhas: zeros(), Outros: zeros(), Rejeitados: zeros(),
		Desperdicados: zeros(), Erros: zeros(), ServicoSoma: rep(4), ServicoN: rep(100),
		Ocupados: rep(4), NoSistema: rep(5), VUs: rep(10), Descartados: zeros(),
		Limites: []float64{0.05, math.Inf(1)}, Buckets: [][]float64{rep(100), rep(100)},
	}
	// Falta nas linhas 7 e 8; depois o sistema fica preso: goodput zero,
	// 400 tentativas por segundo chegando, 200 desperdiçadas.
	for k := 7; k < n; k++ {
		s.Sucesso[k], s.Uteis[k] = 0, 0
		s.Recebidos[k], s.Desperdicados[k], s.Rejeitados[k], s.Fila[k] = 400, 200, 200, 64
	}
	r := resumir(cfg, Cenario{ID: "x", Politica: "immediate", MaxRetries: 3}, s)
	if r.RegimeGoodput != 100 || r.RegimeRazao != 1 || r.RegimeAmplificacao != 1 {
		t.Fatalf("regime: %+v", r)
	}
	// Little: L = 5; λ·W = 100/s × 50 ms = 5 → erro 0.
	if !quase(r.LittleL, 5, 1e-9) || !quase(r.LittleErro, 0, 1e-9) {
		t.Fatalf("Little: L=%v erro=%v", r.LittleL, r.LittleErro)
	}
	if r.Recuperou || r.TempoRecuperacaoS != -1 || r.Desfecho != DesfechoMetaestavel {
		t.Fatalf("devia estar preso: %+v", r)
	}
	if r.FinalGoodput != 0 || r.FinalFracao != 0 || r.FinalAmplificacao != 4 || r.FinalDesperdicio != 200 || r.FinalFila != 64 {
		t.Fatalf("janela final: %+v", r)
	}
	if !quase(r.AtrasoServicoMs, 40, 1e-9) {
		t.Fatalf("tempo de serviço no atraso: %v", r.AtrasoServicoMs)
	}

	// Se o sistema já estava no chão antes do atraso, não há regime para o
	// qual voltar: nada de "recuperou", por menor que seja o alvo.
	semRegime := s
	semRegime.Sucesso = make([]float64, n)
	semRegime.Sucesso[3] = 1 // um único sucesso no regime: razão minúscula, mas positiva
	semRegime.Sucesso[12] = 5
	r = resumir(cfg, Cenario{ID: "z", Politica: "immediate", MaxRetries: 3}, semRegime)
	if r.Recuperou || r.TempoRecuperacaoS != -1 || r.Desfecho != DesfechoSemRegime {
		t.Fatalf("sem regime não pode contar como recuperado: %+v", r)
	}

	// Mesmo cenário, mas o goodput volta 2 s depois do fim da falta.
	for k := 11; k < n; k++ {
		s.Sucesso[k], s.Recebidos[k], s.Desperdicados[k], s.Rejeitados[k], s.Fila[k] = 100, 100, 0, 0, 1
	}
	r = resumir(cfg, Cenario{ID: "y", Politica: "none"}, s)
	if !r.Recuperou || r.TempoRecuperacaoS != 2 || r.Desfecho != DesfechoRecuperou {
		t.Fatalf("devia recuperar em 2 s: tempo=%d desfecho=%s", r.TempoRecuperacaoS, r.Desfecho)
	}
	if r.FinalRazao != 1 || r.FinalFracao != 1 || r.FinalAmplificacao != 1 {
		t.Fatalf("janela final recuperada: %+v", r)
	}
}

func TestJSONComNaN(t *testing.T) {
	r := Resumo{Cenario: Cenario{ID: "a", Politica: "none", CargaPct: 60}, RegimeGoodput: 117.5, FinalP99Ms: nan, LittleErro: math.Inf(1)}
	b, err := codificarJSON(Manifesto{Cenario: r.Cenario, Resumo: r, Colunas: []string{"t_s"}})
	if err != nil {
		t.Fatal(err)
	}
	txt := string(b)
	if strings.Contains(txt, "NaN") || strings.Contains(txt, "Inf") {
		t.Fatalf("NaN/Inf vazou para o JSON:\n%s", txt)
	}
	var volta map[string]any
	if err := json.Unmarshal(b, &volta); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, txt)
	}
	res := volta["resumo"].(map[string]any)
	if res["final_p99_ms"] != nil || res["little_erro_pct"] != nil {
		t.Fatalf("NaN devia virar null: %v", res)
	}
	if res["regime_goodput_s"].(float64) != 117.5 || res["id"].(string) != "a" {
		t.Fatalf("campos da struct embutida ou valores perdidos: %v", res)
	}
	// A ordem dos campos segue a da struct: cenario vem antes de config.
	if strings.Index(txt, `"cenario"`) > strings.Index(txt, `"config"`) {
		t.Fatal("a ordem dos campos não foi preservada")
	}

	// Ida e volta pelo disco: null volta a ser NaN.
	dir := t.TempDir()
	caminho := filepath.Join(dir, "manifesto.json")
	if err := os.WriteFile(caminho, b, 0o644); err != nil {
		t.Fatal(err)
	}
	lido, ok := lerResumo(caminho)
	if !ok {
		t.Fatal("lerResumo não leu o manifesto")
	}
	if !math.IsNaN(lido.FinalP99Ms) || lido.RegimeGoodput != 117.5 || lido.ID != "a" || lido.CargaPct != 60 {
		t.Fatalf("resumo relido errado: %+v", lido)
	}
	if _, ok := lerResumo(filepath.Join(dir, "nao-existe.json")); ok {
		t.Fatal("arquivo inexistente não pode contar como execução feita")
	}
}

func TestVigiaPausas(t *testing.T) {
	// Ambiente tranquilo: o excesso fica bem abaixo do limite.
	v := vigiarPausas()
	time.Sleep(200 * time.Millisecond)
	if maior := v.Parar(); maior > limitePausa {
		t.Skipf("máquina de build instável demais para este teste: pausa de %v", maior)
	}
}

func TestResumosDoDiretorio(t *testing.T) {
	raiz := t.TempDir()
	for _, id := range []string{"immediate-r1-c50-rep1", "backoff-r3-c20-rep1"} {
		dir := filepath.Join(raiz, "execucoes", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		m := Manifesto{Cenario: Cenario{ID: id}, Resumo: Resumo{Cenario: Cenario{ID: id}, Desfecho: DesfechoRecuperou}}
		if err := gravarJSON(filepath.Join(dir, "manifesto.json"), m); err != nil {
			t.Fatal(err)
		}
	}
	// Um diretório sem manifesto (execução interrompida) não conta.
	if err := os.MkdirAll(filepath.Join(raiz, "execucoes", "incompleta"), 0o755); err != nil {
		t.Fatal(err)
	}
	rs := resumosDoDiretorio(raiz)
	if len(rs) != 2 {
		t.Fatalf("esperava 2 resumos, vieram %d", len(rs))
	}
}

func TestResumoCSV(t *testing.T) {
	dir := t.TempDir()
	caminho := filepath.Join(dir, "resumo.csv")
	rs := []Resumo{
		{Cenario: Cenario{ID: "lifo-fixed-r3-c60-rep1", Fila: "lifo", Politica: "fixed", MaxRetries: 3, CargaPct: 60, Rep: 1}, FinalP99Ms: nan},
		{Cenario: Cenario{ID: "fifo-backoff-r3-c90-rep1", Fila: "fifo", Politica: "backoff", CargaPct: 90, Rep: 1}, Recuperou: true, Desfecho: DesfechoRecuperou},
		{Cenario: Cenario{ID: "fifo-fixed-r3-c60-rep1", Fila: "fifo", Politica: "fixed", CargaPct: 60, Rep: 1}, FinalFracImp: 0.5},
	}
	if err := gravarResumoCSV(caminho, rs); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(caminho)
	linhas := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(linhas) != 4 {
		t.Fatalf("esperava cabeçalho + 3 linhas:\n%s", b)
	}
	nomes, _ := camposResumo()
	for i, l := range linhas {
		if got := len(strings.Split(l, ",")); got != len(nomes) {
			t.Fatalf("linha %d tem %d colunas, esperava %d", i, got, len(nomes))
		}
	}
	if !strings.HasPrefix(linhas[0], "id,politica_fila,politica,seed,") {
		t.Fatalf("cabeçalho começa errado: %s", linhas[0][:60])
	}
	for _, col := range []string{"final_fracao_impatient", "descartes_evicted", "atraso_lifo_ativo", "tempo_recuperacao_s"} {
		if !strings.Contains(linhas[0], col) {
			t.Fatalf("faltou a coluna %s", col)
		}
	}
	if strings.Contains(linhas[0], "rotulo") {
		t.Fatal("o rótulo interno não vai para o CSV")
	}
	// Ordem: fila (fifo antes de lifo), cliente (fixed antes de backoff), carga.
	if !strings.HasPrefix(linhas[1], "fifo-fixed") || !strings.HasPrefix(linhas[2], "fifo-backoff") || !strings.HasPrefix(linhas[3], "lifo-fixed") {
		t.Fatalf("ordem errada:\n%s", b)
	}
	if strings.Contains(string(b), "NaN") || !strings.Contains(linhas[1], "0.500") {
		t.Fatalf("valores mal formatados:\n%s", b)
	}
}
