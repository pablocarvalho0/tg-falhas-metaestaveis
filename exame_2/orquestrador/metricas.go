package main

import (
	"math"
	"sort"
)

// Este arquivo só tem funções puras sobre séries de 1 s. É o que os testes
// unitários cobrem: tudo o que vira número no relatório passa por aqui.

// Fases de uma execução, na ordem em que acontecem.
const (
	FaseAquecimento = "aquecimento"
	FaseRegime      = "regime"
	FaseAtraso      = "atraso"
	FaseRecuperacao = "recuperacao"
)

// Fases são as durações, em segundos inteiros, de cada fase.
type Fases struct {
	Aquecimento int `json:"aquecimento_s"`
	Regime      int `json:"regime_s"`
	Atraso      int `json:"atraso_s"`
	Recuperacao int `json:"recuperacao_s"`
}

func (f Fases) Total() int { return f.Aquecimento + f.Regime + f.Atraso + f.Recuperacao }

// Intervalo [ini, fim) em índices de linha. A linha k (k ≥ 1) cobre o
// intervalo de tempo (k-1, k] segundos depois de T0, então a fase que vai de
// a até b segundos corresponde às linhas a+1 .. b.
type Intervalo struct{ Ini, Fim int }

func (f Fases) Linhas(fase string) Intervalo {
	a := 0
	switch fase {
	case FaseAquecimento:
		return Intervalo{1, f.Aquecimento + 1}
	case FaseRegime:
		a = f.Aquecimento
		return Intervalo{a + 1, a + f.Regime + 1}
	case FaseAtraso:
		a = f.Aquecimento + f.Regime
		return Intervalo{a + 1, a + f.Atraso + 1}
	case FaseRecuperacao:
		a = f.Aquecimento + f.Regime + f.Atraso
		return Intervalo{a + 1, a + f.Recuperacao + 1}
	}
	return Intervalo{}
}

// FaseDaLinha diz a que fase pertence a linha k.
func (f Fases) FaseDaLinha(k int) string {
	for _, fase := range []string{FaseAquecimento, FaseRegime, FaseAtraso, FaseRecuperacao} {
		iv := f.Linhas(fase)
		if k >= iv.Ini && k < iv.Fim {
			return fase
		}
	}
	return ""
}

// preencher troca os buracos de uma série acumulada: NaN no começo vira 0
// (o contador ainda não existia) e NaN no meio repete o último valor.
func preencher(cum []float64) []float64 {
	out := make([]float64, len(cum))
	ultimo := 0.0
	for i, v := range cum {
		if !math.IsNaN(v) {
			ultimo = v
		}
		out[i] = ultimo
	}
	return out
}

// lacunaMaxima é o maior intervalo, em segundos, entre duas amostras ainda
// tratado como vizinhança (as amostras chegam a cada 1 s).
const lacunaMaxima = 1.5

// Amostra de um contador acumulado: instante (segundos Unix) e valor.
type amostraBruta struct{ T, V float64 }

// interpolar devolve o valor de um contador acumulado em t0, t0+1, ...,
// t0+n-1 s, por interpolação linear entre as amostras vizinhas.
//
// As amostras chegam a cada 1 s, mas não nos instantes da grade: o k6 envia
// no ritmo dele e o Prometheus coleta no dele. Pegar "a última amostra antes
// de cada instante" faz o que chegou um pouco depois da virada do segundo
// cair no segundo seguinte, e a série fica alternando entre quase zero e o
// dobro. Interpolar o acumulado distribui cada amostra entre os dois
// segundos que ela cobre; a soma não muda.
//
// Antes da primeira amostra: se zeroAntes, o contador ainda não existia e
// vale 0, subindo linearmente até a primeira amostra a partir de 1 s antes
// dela (contadores do k6, que nascem com a execução); senão vale o da
// primeira amostra (contadores do serviço, que já existiam). Depois da
// última amostra o valor fica constante. Sem amostras, tudo NaN.
func interpolar(amostras []amostraBruta, t0 float64, n int, zeroAntes bool) []float64 {
	out := make([]float64, n)
	if len(amostras) == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}
	a := append([]amostraBruta(nil), amostras...)
	sort.Slice(a, func(i, j int) bool { return a[i].T < a[j].T })
	if zeroAntes {
		a = append([]amostraBruta{{a[0].T - 1, 0}}, a...)
	}
	j := 0
	for i := 0; i < n; i++ {
		t := t0 + float64(i)
		for j+1 < len(a) && a[j+1].T <= t {
			j++
		}
		switch {
		case t <= a[0].T:
			out[i] = a[0].V
		case j+1 >= len(a):
			out[i] = a[len(a)-1].V
		default:
			x0, x1 := a[j], a[j+1]
			// Lacuna: o k6 só envia a amostra de um contador quando ele muda.
			// Um contador parado por 20 s e que depois sobe não subiu aos
			// poucos durante os 20 s: ficou parado e subiu no último intervalo.
			if x1.T-x0.T > lacunaMaxima {
				x0 = amostraBruta{x1.T - 1, x0.V}
			}
			switch {
			case t <= x0.T:
				out[i] = x0.V
			case x1.V < x0.V || x1.T == x0.T: // reinício do contador: não interpola
				out[i] = x0.V
			default:
				out[i] = x0.V + (x1.V-x0.V)*(t-x0.T)/(x1.T-x0.T)
			}
		}
	}
	return out
}

// somarSeries soma, ponto a ponto, séries já alinhadas; NaN conta como 0, a
// não ser que todas sejam NaN naquele ponto.
func somarSeries(series [][]float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for _, s := range series {
		for i := 0; i < n && i < len(s); i++ {
			if math.IsNaN(s[i]) {
				continue
			}
			if math.IsNaN(out[i]) {
				out[i] = 0
			}
			out[i] += s[i]
		}
	}
	return out
}

// deltas transforma um contador acumulado amostrado a cada 1 s em incrementos
// por segundo. out[0] é sempre 0: a linha 0 é só a linha de base em T0.
// Um recuo do contador (reinício do processo) conta como recomeço do zero.
func deltas(cum []float64) []float64 {
	c := preencher(cum)
	out := make([]float64, len(c))
	for i := 1; i < len(c); i++ {
		d := c[i] - c[i-1]
		if d < 0 {
			d = c[i]
		}
		out[i] = d
	}
	return out
}

func soma(v []float64, iv Intervalo) float64 {
	s := 0.0
	for i := iv.Ini; i < iv.Fim && i < len(v); i++ {
		if i >= 0 && !math.IsNaN(v[i]) {
			s += v[i]
		}
	}
	return s
}

func media(v []float64, iv Intervalo) float64 {
	n := 0
	s := 0.0
	for i := iv.Ini; i < iv.Fim && i < len(v); i++ {
		if i >= 0 && !math.IsNaN(v[i]) {
			s += v[i]
			n++
		}
	}
	if n == 0 {
		return math.NaN()
	}
	return s / float64(n)
}

func razao(num, den float64) float64 {
	if den == 0 || math.IsNaN(den) || math.IsNaN(num) {
		return math.NaN()
	}
	return num / den
}

// fracaoAtendida é o goodput relativo num intervalo: respostas dentro do
// prazo ÷ pedidos novos. Como a taxa de chegada é constante por construção
// (carga aberta), comparar essa fração com a do regime é o mesmo critério que
// comparar o goodput em req/s com o do regime, mas sem o ruído de contagem das
// chegadas, que aparece igual no numerador e no denominador.
func fracaoAtendida(sucesso, originados []float64, iv Intervalo) float64 {
	return razao(soma(sucesso, iv), soma(originados, iv))
}

// tempoRecuperacao devolve quantos segundos depois do fim da falta o goodput
// voltou ao regime: o menor d ≥ 0 tal que, nos `janela` segundos seguintes a
// (fimFalta + d), a fração atendida é ≥ limiar × a fração atendida no regime.
// Devolve (-1, false) se isso não acontece até o fim da série.
//
// A janela olha para a frente, e não para trás, para que os segundos da
// própria falta (goodput zero) não entrem na conta.
func tempoRecuperacao(sucesso, originados []float64, fimFalta int, regime, limiar float64, janela int) (int, bool) {
	if regime <= 0 || math.IsNaN(regime) {
		return -1, false
	}
	alvo := limiar * regime
	for d := 0; fimFalta+d+janela < len(sucesso); d++ {
		iv := Intervalo{fimFalta + d + 1, fimFalta + d + 1 + janela}
		if fr := fracaoAtendida(sucesso, originados, iv); !math.IsNaN(fr) && fr >= alvo {
			return d, true
		}
	}
	return -1, false
}

// Histograma acumulado no estilo do Prometheus: limites superiores (le) em
// ordem crescente, o último é +Inf, e contagens acumuladas por limite.
type Histograma struct {
	Limites   []float64
	Contagens []float64
}

// quantil estima o quantil q por interpolação linear dentro do bucket, como
// o histogram_quantile do Prometheus. Devolve NaN se não há observações.
func (h Histograma) quantil(q float64) float64 {
	n := len(h.Limites)
	if n == 0 || len(h.Contagens) != n {
		return math.NaN()
	}
	total := h.Contagens[n-1]
	if total <= 0 || math.IsNaN(total) {
		return math.NaN()
	}
	alvo := q * total
	i := sort.Search(n, func(i int) bool { return h.Contagens[i] >= alvo })
	if i >= n {
		i = n - 1
	}
	if math.IsInf(h.Limites[i], 1) {
		// Caiu no bucket aberto: o melhor que se pode dizer é o maior limite finito.
		if n >= 2 {
			return h.Limites[n-2]
		}
		return math.NaN()
	}
	inferior, abaixo := 0.0, 0.0
	if i > 0 {
		inferior, abaixo = h.Limites[i-1], h.Contagens[i-1]
	}
	noBucket := h.Contagens[i] - abaixo
	if noBucket <= 0 {
		return h.Limites[i]
	}
	return inferior + (h.Limites[i]-inferior)*(alvo-abaixo)/noBucket
}

// histogramaNoIntervalo soma, por bucket, os incrementos das linhas do
// intervalo. porLimite[i] é a série de incrementos por segundo do bucket i.
func histogramaNoIntervalo(limites []float64, porLimite [][]float64, iv Intervalo) Histograma {
	h := Histograma{Limites: limites, Contagens: make([]float64, len(limites))}
	for i := range limites {
		h.Contagens[i] = soma(porLimite[i], iv)
	}
	return h
}

// Classificação do desfecho de uma execução.
const (
	DesfechoRecuperou   = "recuperou"
	DesfechoMetaestavel = "metaestavel"
	DesfechoParcial     = "parcial"
	DesfechoSemRegime   = "sem_regime"
)

// classificar decide o desfecho a partir de duas razões: a fração atendida
// no regime (o sistema estava saudável antes do gatilho?) e a fração atendida
// no final dividida pela do regime (voltou ao que era?).
func classificar(regimeSobreOriginados, finalSobreRegime float64) string {
	switch {
	case math.IsNaN(regimeSobreOriginados) || regimeSobreOriginados < 0.95:
		return DesfechoSemRegime
	case math.IsNaN(finalSobreRegime):
		return DesfechoSemRegime
	case finalSobreRegime >= 0.95:
		return DesfechoRecuperou
	case finalSobreRegime <= 0.50:
		return DesfechoMetaestavel
	default:
		return DesfechoParcial
	}
}

// desvioMaxPct é o maior afastamento relativo da média, em %.
func desvioMaxPct(v []float64) (mediaV, desvio float64) {
	if len(v) == 0 {
		return math.NaN(), math.NaN()
	}
	for _, x := range v {
		mediaV += x
	}
	mediaV /= float64(len(v))
	for _, x := range v {
		if d := math.Abs(x-mediaV) / mediaV * 100; d > desvio {
			desvio = d
		}
	}
	return
}
