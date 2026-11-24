package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var httpc = &http.Client{Timeout: 10 * time.Second}

// ---------------------------------------------------------------- Toxiproxy

// Toxiproxy fala com a API HTTP do Toxiproxy. A falta injetada é sempre a
// mesma: latência fixa nas respostas do banco (stream "downstream").
type Toxiproxy struct {
	URL   string
	Proxy string
}

const nomeToxico = "atraso"

func (t Toxiproxy) fazer(metodo, caminho string, corpo any) (int, []byte, error) {
	var r io.Reader
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			return 0, nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(metodo, t.URL+caminho, r)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

// Limpar remove qualquer tóxico ativo no proxy.
func (t Toxiproxy) Limpar() error {
	st, b, err := t.fazer("GET", "/proxies/"+t.Proxy+"/toxics", nil)
	if err != nil {
		return err
	}
	if st != 200 {
		return fmt.Errorf("toxiproxy: listar tóxicos: %d %s", st, b)
	}
	var toxicos []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &toxicos); err != nil {
		return err
	}
	for _, tx := range toxicos {
		st, b, err := t.fazer("DELETE", "/proxies/"+t.Proxy+"/toxics/"+tx.Name, nil)
		if err != nil {
			return err
		}
		if st != 204 {
			return fmt.Errorf("toxiproxy: remover %s: %d %s", tx.Name, st, b)
		}
	}
	return nil
}

// LigarAtraso acrescenta ms de latência a cada resposta do banco.
func (t Toxiproxy) LigarAtraso(ms int) error {
	st, b, err := t.fazer("POST", "/proxies/"+t.Proxy+"/toxics", map[string]any{
		"name":       nomeToxico,
		"type":       "latency",
		"stream":     "downstream",
		"toxicity":   1.0,
		"attributes": map[string]any{"latency": ms, "jitter": 0},
	})
	if err != nil {
		return err
	}
	if st != 200 {
		return fmt.Errorf("toxiproxy: ligar atraso: %d %s", st, b)
	}
	return nil
}

// DesligarAtraso remove a latência.
func (t Toxiproxy) DesligarAtraso() error {
	st, b, err := t.fazer("DELETE", "/proxies/"+t.Proxy+"/toxics/"+nomeToxico, nil)
	if err != nil {
		return err
	}
	if st != 204 {
		return fmt.Errorf("toxiproxy: desligar atraso: %d %s", st, b)
	}
	return nil
}

// Versao devolve a versão do Toxiproxy (vai para o manifesto).
func (t Toxiproxy) Versao() string {
	st, b, err := t.fazer("GET", "/version", nil)
	if err != nil || st != 200 {
		return ""
	}
	var v struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &v) == nil && v.Version != "" {
		return v.Version
	}
	return strings.Trim(strings.TrimSpace(string(b)), `"`)
}

// --------------------------------------------------------------- Prometheus

// Prometheus consulta séries já armazenadas.
type Prometheus struct{ URL string }

// SeriePrometheus é uma série devolvida por query_range, já alinhada: Valores[k]
// é o valor em inicio + k·1s (NaN se não havia amostra).
type SeriePrometheus struct {
	Rotulos map[string]string
	Valores []float64
}

// Consultar avalia a expressão em inicio, inicio+1s, ..., inicio+(n-1)s.
func (p Prometheus) Consultar(expr string, inicio time.Time, n int) ([]SeriePrometheus, error) {
	q := url.Values{}
	q.Set("query", expr)
	t0 := float64(inicio.UnixNano()) / 1e9
	q.Set("start", strconv.FormatFloat(t0, 'f', 3, 64))
	q.Set("end", strconv.FormatFloat(t0+float64(n-1), 'f', 3, 64))
	q.Set("step", "1")
	resp, err := httpc.Get(p.URL + "/api/v1/query_range?" + q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	corpo, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var r struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Values [][2]any          `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(corpo, &r); err != nil {
		return nil, fmt.Errorf("prometheus: resposta inválida para %q: %w", expr, err)
	}
	if r.Status != "success" {
		return nil, fmt.Errorf("prometheus: %q: %s", expr, r.Error)
	}
	var out []SeriePrometheus
	for _, res := range r.Data.Result {
		s := SeriePrometheus{Rotulos: res.Metric, Valores: make([]float64, n)}
		for i := range s.Valores {
			s.Valores[i] = math.NaN()
		}
		for _, par := range res.Values {
			ts, ok1 := par[0].(float64)
			txt, ok2 := par[1].(string)
			if !ok1 || !ok2 {
				continue
			}
			k := int(math.Round(ts - t0))
			v, err := strconv.ParseFloat(txt, 64)
			if err != nil || k < 0 || k >= n {
				continue
			}
			s.Valores[k] = v
		}
		out = append(out, s)
	}
	return out, nil
}

// brutas devolve as amostras cruas (com o instante real de cada uma) de todas
// as séries que casam com o seletor, de 10 s antes de t0 até 5 s depois do
// fim da grade.
func (p Prometheus) brutas(seletor string, t0 time.Time, n int) (rotulos []map[string]string, amostras [][]amostraBruta, err error) {
	q := url.Values{}
	q.Set("query", fmt.Sprintf("%s[%ds]", seletor, n+15))
	fim := float64(t0.UnixNano())/1e9 + float64(n-1) + 5
	q.Set("time", strconv.FormatFloat(fim, 'f', 3, 64))
	resp, err := httpc.Get(p.URL + "/api/v1/query?" + q.Encode())
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	corpo, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	var r struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Values [][2]any          `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(corpo, &r); err != nil {
		return nil, nil, fmt.Errorf("prometheus: resposta inválida para %q: %w", seletor, err)
	}
	if r.Status != "success" {
		return nil, nil, fmt.Errorf("prometheus: %q: %s", seletor, r.Error)
	}
	for _, res := range r.Data.Result {
		var as []amostraBruta
		for _, par := range res.Values {
			ts, ok1 := par[0].(float64)
			txt, ok2 := par[1].(string)
			if !ok1 || !ok2 {
				continue
			}
			if v, err := strconv.ParseFloat(txt, 64); err == nil && !math.IsNaN(v) {
				as = append(as, amostraBruta{ts, v})
			}
		}
		rotulos = append(rotulos, res.Metric)
		amostras = append(amostras, as)
	}
	return rotulos, amostras, nil
}

// Acumulado devolve o valor de um contador em t0, t0+1 s, ..., interpolado
// entre as amostras reais (ver interpolar), somando todas as séries que casam
// com o seletor. Se nenhuma casa, devolve tudo NaN.
func (p Prometheus) Acumulado(seletor string, t0 time.Time, n int, zeroAntes bool) ([]float64, error) {
	_, amostras, err := p.brutas(seletor, t0, n)
	if err != nil {
		return nil, err
	}
	ini := float64(t0.UnixNano()) / 1e9
	var series [][]float64
	for _, as := range amostras {
		series = append(series, interpolar(as, ini, n, zeroAntes))
	}
	return somarSeries(series, n), nil
}

// HistogramaInterpolado é Acumulado para um histograma: devolve os limites
// (le) em ordem e, para cada limite, a série acumulada daquele bucket.
func (p Prometheus) HistogramaInterpolado(seletor string, t0 time.Time, n int) ([]float64, [][]float64, error) {
	rotulos, amostras, err := p.brutas(seletor, t0, n)
	if err != nil {
		return nil, nil, err
	}
	ini := float64(t0.UnixNano()) / 1e9
	type bucket struct {
		le  float64
		val []float64
	}
	var bs []bucket
	for i, as := range amostras {
		le, err := strconv.ParseFloat(rotulos[i]["le"], 64)
		if err != nil {
			return nil, nil, fmt.Errorf("prometheus: le inválido %q", rotulos[i]["le"])
		}
		bs = append(bs, bucket{le, interpolar(as, ini, n, false)})
	}
	sort.Slice(bs, func(i, j int) bool { return bs[i].le < bs[j].le })
	limites := make([]float64, len(bs))
	series := make([][]float64, len(bs))
	for i, b := range bs {
		limites[i], series[i] = b.le, b.val
	}
	return limites, series, nil
}

// Escalar é Consultar para expressões que devolvem no máximo uma série.
// Se a série não existe, devolve tudo NaN.
func (p Prometheus) Escalar(expr string, inicio time.Time, n int) ([]float64, error) {
	ss, err := p.Consultar(expr, inicio, n)
	if err != nil {
		return nil, err
	}
	if len(ss) == 0 {
		v := make([]float64, n)
		for i := range v {
			v[i] = math.NaN()
		}
		return v, nil
	}
	if len(ss) > 1 {
		return nil, fmt.Errorf("prometheus: %q devolveu %d séries; esperava uma", expr, len(ss))
	}
	return ss[0].Valores, nil
}

// HistogramaAcumulado devolve os limites (le) em ordem e, para cada limite, a
// série acumulada do contador daquele bucket.
func (p Prometheus) HistogramaAcumulado(expr string, inicio time.Time, n int) ([]float64, [][]float64, error) {
	ss, err := p.Consultar(expr, inicio, n)
	if err != nil {
		return nil, nil, err
	}
	type bucket struct {
		le  float64
		val []float64
	}
	var bs []bucket
	for _, s := range ss {
		txt := s.Rotulos["le"]
		le, err := strconv.ParseFloat(txt, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("prometheus: le inválido %q", txt)
		}
		bs = append(bs, bucket{le, s.Valores})
	}
	sort.Slice(bs, func(i, j int) bool { return bs[i].le < bs[j].le })
	limites := make([]float64, len(bs))
	series := make([][]float64, len(bs))
	for i, b := range bs {
		limites[i], series[i] = b.le, b.val
	}
	return limites, series, nil
}

// ------------------------------------------------- /metrics lido diretamente

// Amostra é uma leitura instantânea de /metrics do serviço, com o instante em
// que foi feita. Usada na calibração de μ, que não depende do Prometheus.
type Amostra struct {
	Quando  time.Time
	Valores map[string]float64
}

// lerMetricas busca /metrics e devolve todas as amostras sem tratamento:
// a chave é o nome com as labels, exatamente como no texto.
func lerMetricas(base string) (Amostra, error) {
	resp, err := httpc.Get(base + "/metrics")
	if err != nil {
		return Amostra{}, err
	}
	defer resp.Body.Close()
	quando := time.Now()
	if resp.StatusCode != 200 {
		return Amostra{}, fmt.Errorf("%s/metrics: status %d", base, resp.StatusCode)
	}
	a := Amostra{Quando: quando, Valores: map[string]float64{}}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		linha := sc.Text()
		if linha == "" || linha[0] == '#' {
			continue
		}
		i := strings.LastIndexByte(linha, ' ')
		if i < 0 {
			continue
		}
		v, err := strconv.ParseFloat(linha[i+1:], 64)
		if err != nil {
			continue
		}
		a.Valores[linha[:i]] = v
	}
	return a, sc.Err()
}

// Soma soma todas as séries de uma métrica, quaisquer que sejam as labels.
func (a Amostra) Soma(nome string) float64 {
	t := 0.0
	for k, v := range a.Valores {
		if k == nome || strings.HasPrefix(k, nome+"{") {
			t += v
		}
	}
	return t
}

// Concluidos soma os pedidos processados, de todos os desfechos e classes.
func (a Amostra) Concluidos() float64 { return a.Soma("svc_requests_completed_total") }

// definirPolitica troca a política de fila do serviço pelo endpoint de
// administração e confere que a troca valeu.
func definirPolitica(base, nome string, cfg Config) error {
	q := url.Values{}
	q.Set("name", nome)
	if cfg.Detector != "" {
		q.Set("detector", cfg.Detector)
	}
	for k, v := range map[string]float64{"s_hat": cfg.SHatMs, "margin": cfg.MargemMs, "max_wait": cfg.MaxWaitMs, "window": cfg.JanelaMs} {
		if v > 0 {
			q.Set(k, strconv.FormatFloat(v, 'f', -1, 64))
		}
	}
	resp, err := httpc.Post(base+"/admin/policy?"+q.Encode(), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	corpo, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("trocar política para %q: %d %s", nome, resp.StatusCode, corpo)
	}
	var e struct {
		Policy struct {
			Nome string `json:"policy"`
		} `json:"policy"`
	}
	if err := json.Unmarshal(corpo, &e); err != nil || e.Policy.Nome != nome {
		return fmt.Errorf("o serviço não confirmou a política %q: %s", nome, corpo)
	}
	return nil
}

// aguardarOcioso espera o serviço esvaziar (nenhum pedido em fila nem em
// serviço) por pelo menos 1 s seguido.
func aguardarOcioso(base string, limite time.Duration) error {
	fim := time.Now().Add(limite)
	ocioso := 0
	for time.Now().Before(fim) {
		a, err := lerMetricas(base)
		if err == nil && a.Valores["svc_in_system"] == 0 {
			ocioso++
			if ocioso >= 5 {
				return nil
			}
		} else {
			ocioso = 0
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("serviço %s não esvaziou em %v", base, limite)
}

// aguardarPronto espera um endpoint HTTP responder 200.
func aguardarPronto(u string, limite time.Duration) error {
	fim := time.Now().Add(limite)
	var ultimo error
	for time.Now().Before(fim) {
		resp, err := httpc.Get(u)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
		ultimo = err
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%s não ficou pronto em %v: %v", u, limite, ultimo)
}
