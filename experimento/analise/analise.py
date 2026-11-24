"""Análise do experimento 2: políticas de fila × políticas de cliente.

Lê resultados/<varredura>/resumo.csv e as séries de cada execução e gera:

  figuras/<varredura>_carga<NN>.png   grade política de fila × cliente: goodput e desperdício no tempo
  tabelas_<varredura>.md              tabelas por configuração, no total e por classe de cliente

Nenhum número é digitado à mão: tudo sai daqui.
"""

import argparse
import csv
import math
import os
import sys
from collections import defaultdict

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
from matplotlib.lines import Line2D
from matplotlib.patches import Patch

SUPERFICIE = "#fcfcfb"
TINTA = "#0b0b0b"
TINTA_2 = "#52514e"
APAGADO = "#898781"
GRADE = "#e1e0d9"
EIXO = "#c3c2b7"
COR_GOODPUT = "#2a78d6"
COR_DESPERDICIO = "#eb6834"
COR_LIFO = "#1baf7a"

FILAS = ["fifo", "timebound", "deadline", "lifo"]
NOME_FILA = {"fifo": "1 · Fila fixa", "timebound": "2 · Tempo fixo (W)", "deadline": "3 · Prazo por pedido", "lifo": "4 · LIFO adaptativo"}
CLIENTES = ["none", "immediate", "fixed", "backoff"]
NOME_CLIENTE = {"none": "Sem retry", "immediate": "Retry imediato", "fixed": "Espera fixa", "backoff": "Backoff + jitter"}
TEXTO = {"id", "politica_fila", "politica", "desfecho", "recuperou"}


def estilo():
    plt.rcParams.update({
        "figure.facecolor": SUPERFICIE, "axes.facecolor": SUPERFICIE, "savefig.facecolor": SUPERFICIE,
        "font.family": "sans-serif", "font.size": 10, "text.color": TINTA, "axes.labelcolor": TINTA_2,
        "axes.edgecolor": EIXO, "axes.linewidth": 0.8, "axes.grid": True, "axes.axisbelow": True,
        "grid.color": GRADE, "grid.linewidth": 0.8, "xtick.color": APAGADO, "ytick.color": APAGADO,
        "xtick.labelcolor": TINTA_2, "ytick.labelcolor": TINTA_2, "xtick.labelsize": 9, "ytick.labelsize": 9,
        "axes.spines.top": False, "axes.spines.right": False, "lines.linewidth": 2,
        "lines.solid_capstyle": "round", "legend.frameon": False,
    })


def num(txt):
    try:
        return float(txt)
    except (TypeError, ValueError):
        return math.nan


def ler_resumo(raiz):
    caminho = os.path.join(raiz, "resumo.csv")
    if not os.path.exists(caminho):
        return []
    out = []
    with open(caminho, newline="", encoding="utf-8") as f:
        for r in csv.DictReader(f):
            d = {k: (v if k in TEXTO else num(v)) for k, v in r.items()}
            d["recuperou"] = r["recuperou"] == "true"
            d["_dir"] = os.path.join(raiz, "execucoes", r["id"])
            out.append(d)
    return out


def ler_series(r):
    with open(os.path.join(r["_dir"], "series.csv"), newline="", encoding="utf-8") as f:
        linhas = list(csv.DictReader(f))
    s = {"fase": [x["fase"] for x in linhas]}
    for col in linhas[0]:
        if col != "fase":
            s[col] = np.array([num(x[col]) for x in linhas])
    return s


def media(rs, campo):
    v = [r[campo] for r in rs if not math.isnan(r.get(campo, math.nan))]
    return sum(v) / len(v) if v else math.nan


def f(v, casas=1, suf=""):
    if v is None or (isinstance(v, float) and math.isnan(v)):
        return "—"
    return f"{v:.{casas}f}".replace(".", ",") + suf


def tabela(cab, linhas, alin=None):
    alin = alin or ["---"] * 3 + ["---:"] * (len(cab) - 3)
    out = ["| " + " | ".join(cab) + " |", "| " + " | ".join(alin) + " |"]
    out += ["| " + " | ".join(l) + " |" for l in linhas]
    return "\n".join(out) + "\n"


def com_regime(rs):
    return [r for r in rs if r["desfecho"] != "sem_regime"]


def tempos(rs, campo="tempo_recuperacao_s"):
    t = [int(r[campo]) for r in rs if not math.isnan(r[campo]) and r[campo] >= 0]
    if len(t) == len(rs) and t:
        return " / ".join(str(x) for x in t) + " s"
    if not t:
        return "não"
    return f"{len(t)} de {len(rs)}"


# ------------------------------------------------------------------ figuras
def fig_carga(resumo, carga, destino):
    filas = [q for q in FILAS if any(r["politica_fila"] == q for r in resumo)]
    clientes = [c for c in CLIENTES if any(r["politica"] == c for r in resumo)]
    g = defaultdict(list)
    for r in resumo:
        if r["carga_pct"] == carga:
            g[(r["politica_fila"], r["politica"])].append(r)
    fig, eixos = plt.subplots(len(filas), len(clientes), figsize=(4.4 * len(clientes) + 1.4, 2.1 * len(filas) + 1.4),
                              sharex=True, sharey=True, squeeze=False)
    for i, q in enumerate(filas):
        for j, c in enumerate(clientes):
            ax = eixos[i][j]
            rs = g.get((q, c), [])
            if not rs:
                ax.set_visible(False)
                continue
            series = [ler_series(r) for r in rs]
            n = min(len(s["t_s"]) for s in series)
            t = series[0]["t_s"][:n]
            fases = series[0]["fase"]
            ts_atraso = [tt for tt, fa in zip(series[0]["t_s"], fases) if fa == "atraso"]
            if ts_atraso:
                ax.axvspan(min(ts_atraso) - 1, max(ts_atraso), color=APAGADO, alpha=0.14, lw=0, zorder=0)
            ori = np.nanmean([s["originados"][:n] for s in series], axis=0)
            ax.plot(t, ori, color=EIXO, lw=1, zorder=1)
            for col, cor in (("desperdicados", COR_DESPERDICIO), ("sucesso", COR_GOODPUT)):
                m = np.vstack([s[col][:n] for s in series])
                ax.fill_between(t, np.nanmin(m, axis=0), np.nanmax(m, axis=0), color=cor, alpha=0.16, lw=0, zorder=2)
                ax.plot(t, np.nanmean(m, axis=0), color=cor, zorder=3)
            if q == "lifo" and "lifo" in series[0]:
                lifo = np.nanmean([s["lifo"][:n] for s in series], axis=0)
                ax.fill_between(t, 0, lifo * 12, color=COR_LIFO, alpha=0.5, lw=0, step="mid", zorder=1)
            ok = sum(1 for r in rs if r["desfecho"] == "recuperou")
            antes = sum(1 for r in rs if r["desfecho"] == "sem_regime")
            nota = f"{ok} de {len(rs)} recuperaram" + (f"\n{antes} caíram antes do atraso" if antes else "")
            ax.text(0.98, 0.66, nota, transform=ax.transAxes, ha="right", va="center", fontsize=8.5, color=TINTA_2)
            if i == 0:
                ax.set_title(NOME_CLIENTE[c], fontsize=10.5, color=TINTA, pad=8)
            if j == 0:
                ax.set_ylabel(NOME_FILA[q] + "\n\npedidos/s", fontsize=9)
            if i == len(filas) - 1:
                ax.set_xlabel("tempo (s)")
            ax.set_xlim(0, t[-1])
            ax.set_ylim(0, max(230, float(np.nanmax(ori)) * 1.25))
    fig.legend(handles=[
        Line2D([], [], color=COR_GOODPUT, lw=2, label="goodput (respostas no prazo)"),
        Line2D([], [], color=COR_DESPERDICIO, lw=2, label="trabalho desperdiçado"),
        Line2D([], [], color=EIXO, lw=1, label="pedidos novos"),
        Patch(facecolor=APAGADO, alpha=0.14, label="atraso no banco"),
        Patch(facecolor=COR_LIFO, alpha=0.5, label="modo LIFO ligado"),
    ], loc="upper center", ncol=5 if len(clientes) > 1 else 2, bbox_to_anchor=(0.5, 0.965), fontsize=9)
    fig.suptitle(f"Políticas de fila × clientes, carga de {carga:.0f} % de μ", x=0.01, ha="left", fontsize=13, fontweight="bold")
    fig.text(0.01, 0.005, "Linha: média das repetições; faixa: mínimo e máximo. Resolução de 1 s.", fontsize=8.5, color=APAGADO)
    fig.tight_layout(rect=(0, 0.02, 1, 0.93 if len(clientes) > 1 else 0.88))
    fig.savefig(destino, dpi=150)
    plt.close(fig)


def fig_politica(resumo, fila, destino):
    """Uma política de fila: linhas = carga, colunas = cliente."""
    cargas = sorted({r["carga_pct"] for r in resumo})
    clientes = [c for c in CLIENTES if any(r["politica"] == c for r in resumo)]
    g = defaultdict(list)
    for r in resumo:
        if r["politica_fila"] == fila:
            g[(r["carga_pct"], r["politica"])].append(r)
    fig, eixos = plt.subplots(len(cargas), len(clientes), figsize=(4.6 * len(clientes) + 1.4, 2.1 * len(cargas) + 1.5),
                              sharex=True, squeeze=False)
    for i, carga in enumerate(cargas):
        for j, c in enumerate(clientes):
            ax = eixos[i][j]
            rs = g.get((carga, c), [])
            if not rs:
                ax.set_visible(False)
                continue
            series = [ler_series(r) for r in rs]
            n = min(len(s["t_s"]) for s in series)
            t = series[0]["t_s"][:n]
            ts_atraso = [tt for tt, fa in zip(series[0]["t_s"], series[0]["fase"]) if fa == "atraso"]
            if ts_atraso:
                ax.axvspan(min(ts_atraso) - 1, max(ts_atraso), color=APAGADO, alpha=0.14, lw=0, zorder=0)
            ori = np.nanmean([s["originados"][:n] for s in series], axis=0)
            ax.plot(t, ori, color=EIXO, lw=1, zorder=1)
            for col, cor in (("desperdicados", COR_DESPERDICIO), ("sucesso", COR_GOODPUT)):
                m = np.vstack([s[col][:n] for s in series])
                ax.fill_between(t, np.nanmin(m, axis=0), np.nanmax(m, axis=0), color=cor, alpha=0.16, lw=0, zorder=2)
                ax.plot(t, np.nanmean(m, axis=0), color=cor, zorder=3)
            if fila == "lifo" and "lifo" in series[0]:
                lifo = np.nanmean([s["lifo"][:n] for s in series], axis=0)
                ax.fill_between(t, 0, lifo * 12, color=COR_LIFO, alpha=0.5, lw=0, step="mid", zorder=1)
            ok = sum(1 for r in rs if r["desfecho"] == "recuperou")
            antes = sum(1 for r in rs if r["desfecho"] == "sem_regime")
            nota = f"{ok} de {len(rs)} recuperaram" + (f"\n{antes} caíram antes do atraso" if antes else "")
            ax.text(0.98, 0.93, nota, transform=ax.transAxes, ha="right", va="top", fontsize=8.5, color=TINTA_2)
            if i == 0:
                ax.set_title(NOME_CLIENTE[c], fontsize=10.5, color=TINTA, pad=8)
            if j == 0:
                ax.set_ylabel(f"carga {carga:.0f} % de μ\n\npedidos/s", fontsize=9)
            if i == len(cargas) - 1:
                ax.set_xlabel("tempo (s)")
            ax.set_xlim(0, t[-1])
            ax.set_ylim(0, 230)
    fig.legend(handles=[
        Line2D([], [], color=COR_GOODPUT, lw=2, label="goodput (respostas no prazo)"),
        Line2D([], [], color=COR_DESPERDICIO, lw=2, label="trabalho desperdiçado"),
        Line2D([], [], color=EIXO, lw=1, label="pedidos novos"),
        Patch(facecolor=APAGADO, alpha=0.14, label="atraso no banco"),
    ] + ([Patch(facecolor=COR_LIFO, alpha=0.5, label="modo LIFO ligado")] if fila == "lifo" else []),
        loc="upper center", ncol=5, bbox_to_anchor=(0.5, 0.955), fontsize=9)
    fig.suptitle(f"Política de fila {NOME_FILA[fila]}", x=0.01, ha="left", fontsize=13, fontweight="bold")
    reps = sorted({int(r["rep"]) for r in resumo})
    fig.text(0.01, 0.005, f"Repetições: {', '.join(map(str, reps))}. Linha: média das repetições; faixa: mínimo e máximo. Resolução de 1 s.",
             fontsize=8.5, color=APAGADO)
    fig.tight_layout(rect=(0, 0.02, 1, 0.92))
    fig.savefig(destino, dpi=150)
    plt.close(fig)


# ------------------------------------------------------------------ tabelas
def chave(r):
    return (FILAS.index(r["politica_fila"]) if r["politica_fila"] in FILAS else 9,
            CLIENTES.index(r["politica"]) if r["politica"] in CLIENTES else 9, r["carga_pct"])


def grupos(resumo):
    g = defaultdict(list)
    for r in resumo:
        g[(r["politica_fila"], r["politica"], r["carga_pct"])].append(r)
    return sorted(g.items(), key=lambda kv: chave(kv[1][0]))


def tab_desfecho(resumo):
    linhas = []
    for (q, c, carga), todas in grupos(resumo):
        rs = com_regime(todas)
        linhas.append([
            NOME_FILA.get(q, q), NOME_CLIENTE.get(c, c), f(carga, 0, " %"), str(len(todas)),
            str(len(todas) - len(rs)),
            f"{sum(1 for r in rs if r['desfecho'] == 'recuperou')} de {len(rs)}",
            tempos(rs), f(media(rs, "final_fracao_atendida") * 100, 1, " %"),
            f(media(rs, "final_amplificacao"), 2), f(media(rs, "final_desperdicados_s"), 1),
            f(media(rs, "final_p99_ms"), 0),
        ])
    return tabela(["Fila", "Cliente", "Carga", "Execuções", "Caíram antes do atraso", "Recuperaram",
                   "Tempo de recuperação", "Atendidos no prazo ao final", "Amplif. final",
                   "Desperdício final (req/s)", "p99 final no serviço (ms)"], linhas)


def tab_classes(resumo):
    linhas = []
    for (q, c, carga), todas in grupos(resumo):
        rs = com_regime(todas)
        linhas.append([
            NOME_FILA.get(q, q), NOME_CLIENTE.get(c, c), f(carga, 0, " %"),
            f(media(rs, "final_fracao_impatient") * 100, 1, " %"), f(media(rs, "final_fracao_patient") * 100, 1, " %"),
            tempos(rs, "tempo_recuperacao_impatient_s"), tempos(rs, "tempo_recuperacao_patient_s"),
            f(media(rs, "final_desperdicados_impatient_s"), 1), f(media(rs, "final_desperdicados_patient_s"), 1),
            f(media(rs, "final_latencia_ok_impatient_ms"), 0), f(media(rs, "final_latencia_ok_patient_ms"), 0),
        ])
    return tabela(["Fila", "Cliente", "Carga", "Atendidos ao final, impatient", "Atendidos ao final, patient",
                   "Recuperação impatient", "Recuperação patient", "Desperdício impatient (req/s)",
                   "Desperdício patient (req/s)", "Latência ok impatient (ms)", "Latência ok patient (ms)"], linhas)


def tab_custo(resumo):
    linhas = []
    for (q, c, carga), todas in grupos(resumo):
        rs = com_regime(todas)
        linhas.append([
            NOME_FILA.get(q, q), NOME_CLIENTE.get(c, c), f(carga, 0, " %"),
            f(media(rs, "regime_fracao_impatient") * 100, 2, " %"), f(media(rs, "regime_fracao_patient") * 100, 2, " %"),
            f(media(rs, "regime_descartes_s"), 2),
            f(media(rs, "regime_descartes_desnecessarios_impatient_s"), 2), f(media(rs, "regime_descartes_desnecessarios_patient_s"), 2),
            f(media(rs, "recuperacao_descartes_desnecessarios_impatient_s"), 2), f(media(rs, "recuperacao_descartes_desnecessarios_patient_s"), 2),
            f(media(rs, "regime_lifo_ativo") * 100, 1, " %"), f(media(rs, "atraso_lifo_ativo") * 100, 0, " %"),
            f(media(rs, "final_lifo_ativo") * 100, 1, " %"),
        ])
    return tabela(["Fila", "Cliente", "Carga", "Atendidos no regime, impatient", "Atendidos no regime, patient",
                   "Descartes no regime (req/s)", "Desnecessários no regime, impatient", "Desnecessários no regime, patient",
                   "Desnecessários na recuperação, impatient", "Desnecessários na recuperação, patient",
                   "LIFO ligado no regime", "LIFO ligado no atraso", "LIFO ligado ao final"], linhas)


def tab_motivos(resumo):
    linhas = []
    for (q, c, carga), todas in grupos(resumo):
        linhas.append([NOME_FILA.get(q, q), NOME_CLIENTE.get(c, c), f(carga, 0, " %")] +
                      [f(media(todas, "descartes_" + m), 0) for m in ("full", "evicted", "expired", "deadline", "stale")])
    return tabela(["Fila", "Cliente", "Carga", "full", "evicted", "expired", "deadline", "stale"], linhas)


def tab_qualidade(resumo):
    pausas = [r["maior_pausa_ambiente_ms"] for r in resumo if not math.isnan(r["maior_pausa_ambiente_ms"])]
    erros = [abs(r["little_erro_pct"]) for r in com_regime(resumo) if not math.isnan(r["little_erro_pct"])]
    return tabela(["Checagem", "Valor"], [
        ["Execuções", str(len(resumo))],
        ["Iterações descartadas pelo k6 (soma)", f(sum(r["iteracoes_descartadas"] for r in resumo), 0)],
        ["Erros do cliente que não são timeout nem 503 (soma)", f(sum(r["outros_erros_cliente"] for r in resumo), 0)],
        ["Erros do serviço ao falar com o banco (soma)", f(sum(r["erros_servidor"] for r in resumo), 0)],
        ["Erro da lei de Little no regime (mediana / máximo)", f"{f(float(np.median(erros)), 2)} % / {f(max(erros), 2)} %" if erros else "—"],
        ["Maior congelamento do ambiente (mediana / máximo)", f"{f(float(np.median(pausas)), 1)} ms / {f(max(pausas), 1)} ms" if pausas else "—"],
    ], ["---", "---:"])


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--resultados", default="/resultados")
    ap.add_argument("--varredura", default="principal")
    ap.add_argument("--reps", default="", help="repetições a usar, ex.: 1 ou 1,2 (padrão: todas)")
    a = ap.parse_args()
    estilo()
    raiz = os.path.join(a.resultados, a.varredura)
    resumo = ler_resumo(raiz)
    if a.reps:
        quero = {int(x) for x in a.reps.split(",")}
        resumo = [r for r in resumo if int(r["rep"]) in quero]
    if not resumo:
        sys.exit(f"não encontrei {raiz}/resumo.csv")
    figs = os.path.join(a.resultados, "figuras")
    os.makedirs(figs, exist_ok=True)
    for carga in sorted({r["carga_pct"] for r in resumo}):
        fig_carga(resumo, carga, os.path.join(figs, f"{a.varredura}_carga{carga:02.0f}.png"))
    # Uma figura por política de fila, em pasta separada.
    pasta = os.path.join(figs, "por_politica")
    os.makedirs(pasta, exist_ok=True)
    for i, fila in enumerate(FILAS, 1):
        if any(r["politica_fila"] == fila for r in resumo):
            fig_politica(resumo, fila, os.path.join(pasta, f"{a.varredura}_{i}_{fila}.png"))
    partes = [
        f"# Tabelas da varredura `{a.varredura}`\n", "Geradas por analise.py; não edite.\n",
        "## Desfecho\n", tab_desfecho(resumo),
        "## Por classe de cliente\n", tab_classes(resumo),
        "## Custo e detector de sobrecarga\n", tab_custo(resumo),
        "## Descartes por motivo (total por execução, média)\n", tab_motivos(resumo),
        "## Qualidade da medição\n", tab_qualidade(resumo),
    ]
    destino = os.path.join(a.resultados, f"tabelas_{a.varredura}.md")
    with open(destino, "w", encoding="utf-8") as arq:
        arq.write("\n".join(partes))
    print(f"figuras em {figs}; tabelas em {destino}")


if __name__ == "__main__":
    main()
