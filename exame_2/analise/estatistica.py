"""Agrega as repetições da varredura: uma linha por (fila, cliente, carga).

Uso: python3 analise/estatistica.py resultados/principal/resumo.csv > resultados/estatistica_principal.md

Para cada célula: n execuções, quantas recuperaram / ficaram presas / caíram
antes da falha, média ± desvio do tempo de recuperação (só das que
recuperaram) e média ± desvio da fração respondida no prazo ao fim.
Só biblioteca padrão.
"""

import csv
import math
import sys
from collections import defaultdict

FILAS = ["fifo", "timebound", "deadline", "lifo"]
NOME = {"fifo": "1 · Fila fixa", "timebound": "2 · Limite fixo", "deadline": "3 · Prazo por pedido", "lifo": "4 · LIFO adaptativo"}


def num(x):
    try:
        return float(x)
    except (TypeError, ValueError):
        return math.nan


def media_dp(v):
    v = [x for x in v if not math.isnan(x)]
    if not v:
        return "—"
    m = sum(v) / len(v)
    if len(v) == 1:
        return f"{m:.1f}"
    dp = math.sqrt(sum((x - m) ** 2 for x in v) / (len(v) - 1))
    return f"{m:.1f} ± {dp:.1f}"


def main(caminho):
    cel = defaultdict(list)
    with open(caminho, newline="", encoding="utf-8") as f:
        for r in csv.DictReader(f):
            if r.get("sem_falta") == "true":
                continue
            cel[(r["politica_fila"], r["politica"], num(r["carga_pct"]))].append(r)
    print("| Fila | Cliente | Carga | n | Recuperou | Preso/parcial | Caiu antes | Recuperação (s) | Respondidos ao fim (%) |")
    print("|---|---|---:|---:|---:|---:|---:|---:|---:|")
    for (fila, cli, carga) in sorted(cel, key=lambda k: (FILAS.index(k[0]) if k[0] in FILAS else 9, k[1], k[2])):
        rs = cel[(fila, cli, carga)]
        rec = [r for r in rs if r["recuperou"] == "true"]
        antes = [r for r in rs if "regime" in r.get("desfecho", "") or "sem" in r.get("desfecho", "")]
        t = [num(r["tempo_recuperacao_s"]) for r in rec]
        fr = [100 * num(r["final_fracao_atendida"]) for r in rs]
        print(f"| {NOME.get(fila, fila)} | {cli} | {carga:.0f} % | {len(rs)} | {len(rec)} | "
              f"{len(rs) - len(rec) - len(antes)} | {len(antes)} | {media_dp(t)} | {media_dp(fr)} |")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
