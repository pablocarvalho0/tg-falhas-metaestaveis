"""Modelo analítico + simulação de eventos discretos da bancada do experimento 2.

Objetivo: prever, sem Docker, o que a bancada mede, e checar se os resultados
fazem sentido pela teoria de filas. Só biblioteca padrão do Python.

Uso:
    python3 modelo-matematico/simulador.py analitico          # contas fechadas (fronteira, esperas, capacidade na falha)
    python3 modelo-matematico/simulador.py grade              # 4 filas x 2 clientes x 4 cargas (mesmo desenho da bancada)
    python3 modelo-matematico/simulador.py fronteira          # fila fixa varrendo a carga de 5 em 5 %: onde o laço fecha?
    python3 modelo-matematico/simulador.py fronteira --svg modelo-matematico/fronteira.svg
    python3 modelo-matematico/simulador.py transicao --reps 10   # fila fixa, 26–40 %: fração de execuções presas

Parâmetros iguais aos da bancada (metodologia.md): C = 8 workers, Q = 64,
Ŝ = 42 ms, falha +200 ms por 20 s, timeouts 100/200 ms (50 % cada), N = 3
retries, W = 98 ms, margem 10 ms, janela do detector LIFO 100 ms.

Simplificações (diferenças em relação à bancada):
  * sem rede: o pedido chega ao servidor no mesmo instante em que sai do cliente;
  * a falha é aplicada ao tempo de serviço de quem COMEÇA durante a janela;
  * o tempo de serviço tem só um ruído pequeno (normal, dp 1,5 ms), sem as
    "engasgadas" do ambiente (congelamentos de até 50 ms) que na bancada
    funcionam como gatilho extra.
"""

import argparse
import heapq
import random
import sys
from collections import deque

# ----------------------------------------------------------------------------- parâmetros
C = 8
Q = 64
S = 0.042           # tempo de serviço (s)
S_DP = 0.0015       # ruído do tempo de serviço
ATRASO = 0.200      # falha: +200 ms no banco
N = 3               # retries por pedido
MARGEM = 0.010
TIMEOUTS = {"impatient": 0.100, "patient": 0.200}
W = (sum(TIMEOUTS.values()) / 2) - S - MARGEM   # 0,098 s
JANELA = 0.100      # detector de sobrecarga do LIFO
MU = C / S          # ~190 req/s

AQUEC, REGIME, FALHA, RECUP = 10, 30, 20, 90
T_FALHA_INI = AQUEC + REGIME
T_FALHA_FIM = T_FALHA_INI + FALHA
T_FIM = T_FALHA_FIM + RECUP


# ----------------------------------------------------------------------------- analítico
def analitico():
    mu_falha = C / (S + ATRASO)
    print("== Contas fechadas (mesmos parâmetros da bancada)")
    print(f"capacidade normal        μ  = C/Ŝ = {C}/{S*1000:.0f} ms = {MU:.1f} req/s (medido: 190,45)")
    print(f"capacidade na falha      μf = C/(Ŝ+200 ms) = {mu_falha:.1f} req/s (methodology: ~33)")
    print(f"espera com a fila cheia  Q/μ = {Q}/{MU:.0f} = {Q/MU*1000:.0f} ms  > timeouts 100 e 200 ms")
    print("  -> com a fila cheia, TODO pedido na fila FIFO estoura o timeout antes de ser atendido.")
    print(f"amplificação no estado preso = N+1 = {N+1} tentativas por pedido novo")
    print(f"fronteira do laço (fila fixa): (N+1)·λ ≥ μ  =>  λ* = μ/(N+1) = {MU/(N+1):.1f} req/s = {100/(N+1):.0f} % de μ")
    print("  -> abaixo de 25 % o sistema preso drena sozinho; acima, os retries sozinhos já saturam o servidor.")
    print(f"limite fixo W = timeout médio − Ŝ − margem = 150 − 42 − 10 = {W*1000:.0f} ms")
    for c, t in TIMEOUTS.items():
        util = t - S - MARGEM
        print(f"  classe {c:9s}: timeout {t*1000:.0f} ms, espera útil {util*1000:.0f} ms "
              f"-> W {'deixa esperar demais (trabalho desperdiçado)' if W > util else 'descarta cedo demais'}")
    print("prazo por pedido: descarta se agora + Ŝ + margem > prazo  -> nunca começa trabalho inútil (salvo ruído > margem)")
    print("custo de um descarte na saída da fila ≈ 0 -> o worker livre limpa todo o prefixo vencido de uma vez")


# ----------------------------------------------------------------------------- simulação
class Sim:
    def __init__(self, fila, cliente, carga_pct, semente):
        self.fila, self.cliente = fila, cliente
        self.lam = carga_pct / 100 * MU
        self.rng = random.Random(semente)
        self.ev = []          # heap (t, seq, tipo, dados)
        self.seq = 0
        self.q = deque()      # itens: [t_enfileirado, prazo, tentativa]
        self.livres = C
        self.ult_vazia = 0.0
        # contadores por segundo
        n = T_FIM + 1
        self.novos = [0] * n
        self.ok = [0] * n          # respostas no prazo (pedido resolvido)
        self.desperd = [0] * n     # trabalho concluído depois que o cliente desistiu
        self.tentativas = [0] * n

    def agendar(self, t, tipo, dados=None):
        self.seq += 1
        heapq.heappush(self.ev, (t, self.seq, tipo, dados))

    # --- cliente
    def espera_retry(self, k):
        if self.cliente == "fixed":
            return 0.050
        return self.rng.uniform(0, min(1.0, 0.100 * 2 ** (k - 1)))

    def enviar(self, t, pedido):
        tent = {"pedido": pedido, "t0": t, "prazo": t + pedido["budget"], "vivo": True}
        pedido["k"] += 1
        self.tentativas[int(t)] += 1
        self.agendar(tent["prazo"], "timeout", tent)
        self.chegar(t, tent)

    def falhou(self, t, tent):
        """503 ou timeout: o cliente tenta de novo, se ainda pode."""
        if not tent["vivo"]:
            return
        tent["vivo"] = False
        p = tent["pedido"]
        if p["k"] <= N:
            self.agendar(t + self.espera_retry(p["k"]), "retry", p)

    # --- servidor
    def marcar_vazia(self, t):
        if not self.q:
            self.ult_vazia = t

    def chegar(self, t, tent):
        self.marcar_vazia(t)
        if len(self.q) >= Q:
            if self.fila == "lifo":
                velho = self.q.popleft()
                self.falhou(t, velho[2])          # despejado
            else:
                self.falhou(t, tent)               # 503 "full"
                return
        self.q.append([t, tent["prazo"], tent])
        self.despachar(t)

    def retirar(self, t):
        """Devolve o próximo item a processar, descartando conforme a política."""
        while self.q:
            if self.fila == "fifo":
                return self.q.popleft()
            if self.fila == "timebound":
                it = self.q.popleft()
                if t - it[0] > W:
                    self.falhou(t, it[2]); continue
                return it
            if self.fila == "deadline":
                it = self.q.popleft()
                if t + S + MARGEM > it[1]:
                    self.falhou(t, it[2]); continue
                return it
            # lifo adaptativo
            sobrecarga = (t - self.ult_vazia) > JANELA
            if sobrecarga:
                while self.q and t - self.q[0][0] > W:
                    self.falhou(t, self.q.popleft()[2])
                if not self.q:
                    break
                it = self.q.pop()
            else:
                it = self.q.popleft()
            if t - it[0] > W:
                self.falhou(t, it[2]); continue
            return it
        return None

    def despachar(self, t):
        while self.livres > 0:
            it = self.retirar(t)
            self.marcar_vazia(t)
            if it is None:
                return
            self.livres -= 1
            s = max(0.001, self.rng.gauss(S, S_DP))
            if T_FALHA_INI <= t < T_FALHA_FIM:
                s += ATRASO
            self.agendar(t + s, "fim", it[2])

    def concluir(self, t, tent):
        self.livres += 1
        seg = min(int(t), T_FIM)
        if tent["vivo"] and t <= tent["prazo"]:
            tent["vivo"] = False
            self.ok[int(tent["pedido"]["t_novo"])] += 1
        else:
            self.desperd[seg] += 1
        self.despachar(t)

    def rodar(self):
        # chegadas abertas: Poisson de taxa λ
        t = 0.0
        while True:
            t += self.rng.expovariate(self.lam)
            if t >= T_FIM:
                break
            classe = "impatient" if self.rng.random() < 0.5 else "patient"
            self.agendar(t, "novo", {"budget": TIMEOUTS[classe], "k": 0, "t_novo": t})
        while self.ev:
            t, _, tipo, d = heapq.heappop(self.ev)
            if t > T_FIM + 2:
                break
            if tipo == "novo":
                self.novos[int(t)] += 1
                self.enviar(t, d)
            elif tipo == "retry":
                if t < T_FIM:
                    self.enviar(t, d)
            elif tipo == "timeout":
                self.falhou(t, d)
            elif tipo == "fim":
                self.concluir(t, d)
        return self.resumo()

    def resumo(self):
        def frac(a, b):
            n = sum(self.novos[a:b])
            return sum(self.ok[a:b]) / n if n else float("nan")
        reg = frac(AQUEC, T_FALHA_INI)
        fim = frac(T_FIM - 30, T_FIM - 1)
        amp = sum(self.tentativas[T_FIM - 30:T_FIM - 1]) / max(1, sum(self.novos[T_FIM - 30:T_FIM - 1]))
        desp = sum(self.desperd[T_FIM - 30:T_FIM - 1]) / 29
        if reg < 0.95:
            desf = "caiu antes da falha"
        elif fim >= 0.95 * reg:
            desf = "recuperou"
        elif fim <= 0.5 * reg:
            desf = "PRESO"
        else:
            desf = "parcial"
        return {"regime": reg, "final": fim, "amp": amp, "desperd": desp, "desfecho": desf}


def grade(reps):
    print(f"{'fila':10s} {'cliente':8s} " + " ".join(f"{c:>26d}%" for c in (20, 50, 70, 90)))
    for fila in ("fifo", "timebound", "deadline", "lifo"):
        for cli in ("fixed", "backoff"):
            cel = []
            for carga in (20, 50, 70, 90):
                rs = [Sim(fila, cli, carga, 1000 * r + carga).rodar() for r in range(reps)]
                fr = sum(x["final"] for x in rs) / reps
                d = "/".join(sorted({x["desfecho"] for x in rs}))
                cel.append(f"{d[:18]:>18s} {fr*100:5.1f}%")
            print(f"{fila:10s} {cli:8s} " + " ".join(f"{c:>27s}" for c in cel))


def fronteira(reps, svg):
    cargas = list(range(5, 100, 5))
    linhas = {}
    print("carga  " + "  ".join(f"{f+'/'+c:>18s}" for f in ("fifo", "deadline") for c in ("fixed", "backoff")))
    for carga in cargas:
        out = []
        for fila in ("fifo", "deadline"):
            for cli in ("fixed", "backoff"):
                rs = [Sim(fila, cli, carga, 77 * r + carga).rodar() for r in range(reps)]
                fr = sum(x["final"] for x in rs) / reps
                linhas.setdefault((fila, cli), []).append(fr)
                out.append(f"{fr*100:6.1f}% amp {sum(x['amp'] for x in rs)/reps:4.2f}")
        print(f"{carga:4d}%  " + "  ".join(f"{o:>18s}" for o in out))
    print(f"previsão analítica da fronteira da fila fixa: {100/(N+1):.0f} %")
    if svg:
        escrever_svg(svg, cargas, linhas)
        print(f"figura: {svg}")


def escrever_svg(caminho, cargas, linhas):
    L, A, m = 760, 420, 60
    x = lambda c: m + (c - 0) / 100 * (L - 2 * m)
    y = lambda f: A - m - max(0, min(1.05, f)) / 1.05 * (A - 2 * m)
    cores = {("fifo", "fixed"): "#d03b3b", ("fifo", "backoff"): "#eb6834",
             ("deadline", "fixed"): "#2a78d6", ("deadline", "backoff"): "#1baf7a"}
    nomes = {"fifo": "fila fixa", "deadline": "prazo por pedido", "fixed": "espera fixa", "backoff": "backoff+jitter"}
    s = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {L} {A}" font-family="sans-serif" font-size="13">',
         f'<rect width="{L}" height="{A}" fill="#fcfcfb"/>']
    for f in (0, 0.25, 0.5, 0.75, 1.0):
        s.append(f'<line x1="{m}" x2="{L-m}" y1="{y(f):.1f}" y2="{y(f):.1f}" stroke="#e1e0d9"/>')
        s.append(f'<text x="{m-8}" y="{y(f)+4:.1f}" text-anchor="end" fill="#52514e">{int(f*100)}%</text>')
    for c in range(0, 101, 10):
        s.append(f'<text x="{x(c):.1f}" y="{A-m+20}" text-anchor="middle" fill="#52514e">{c}%</text>')
    xf = x(100 / (N + 1))
    s.append(f'<line x1="{xf:.1f}" x2="{xf:.1f}" y1="{m-10}" y2="{A-m}" stroke="#0b0b0b" stroke-dasharray="5 4"/>')
    s.append(f'<text x="{xf+6:.1f}" y="{m-14}" fill="#0b0b0b">μ/(N+1) = 25 %</text>')
    for i, (k, v) in enumerate(linhas.items()):
        pts = " ".join(f"{x(c):.1f},{y(f):.1f}" for c, f in zip(cargas, v))
        s.append(f'<polyline points="{pts}" fill="none" stroke="{cores[k]}" stroke-width="2.5"/>')
        s.append(f'<text x="{L-m-230}" y="{y(0.72)+18*i:.1f}" fill="{cores[k]}">— {nomes[k[0]]}, {nomes[k[1]]}</text>')
    s.append(f'<text x="{L/2}" y="{A-14}" text-anchor="middle" fill="#0b0b0b">carga (% de μ)</text>')
    s.append(f'<text x="16" y="{A/2}" transform="rotate(-90 16 {A/2})" text-anchor="middle" fill="#0b0b0b">respondidos no prazo, últimos 30 s</text>')
    s.append("</svg>")
    with open(caminho, "w", encoding="utf-8") as f:
        f.write("\n".join(s))


def transicao(reps):
    print(f"fila fixa: execuções presas em {reps} (sementes diferentes)")
    for carga in range(26, 42, 2):
        out = []
        for cli in ("fixed", "backoff"):
            rs = [Sim("fifo", cli, carga, 991 * r + carga).rodar() for r in range(reps)]
            out.append(f"{cli}: {sum(x['desfecho'] == 'PRESO' for x in rs):2d}/{reps}")
        print(f"{carga:3d}%   " + "   ".join(out))


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("modo", choices=["analitico", "grade", "fronteira", "transicao"])
    ap.add_argument("--reps", type=int, default=1)
    ap.add_argument("--svg")
    a = ap.parse_args()
    if a.modo == "analitico":
        analitico()
    elif a.modo == "grade":
        grade(a.reps)
    elif a.modo == "transicao":
        transicao(a.reps)
    else:
        fronteira(a.reps, a.svg)
