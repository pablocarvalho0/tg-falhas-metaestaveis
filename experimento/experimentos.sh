#!/usr/bin/env bash
# Campanha completa do experimento 2 (plano.md): calibração, varredura principal
# (4 políticas de fila × 2 clientes × 4 cargas × 3 repetições = 96 execuções)
# e análise. Cerca de 4 h 30.
#
# No macOS: laptop NA TOMADA, tampa aberta, e
#     caffeinate -dimsu ./experimentos.sh
# Na bateria o macOS reduz o desempenho: o tempo de serviço sobe de ~42 para
# ~52 ms e faltam coletas do Prometheus (visto nos pilotos de 2026-11-21).
# Se a tela apaga ou o laptop dorme, a máquina virtual congela; o
# orquestrador detecta e repete a execução afetada. Pode ser interrompido e
# rodado de novo: o que já foi feito é reaproveitado.
set -euo pipefail
cd "$(dirname "$0")"

if pmset -g batt 2>/dev/null | grep -q "Battery Power"; then
  echo "ATENÇÃO: o laptop está na bateria; ligue na tomada antes da campanha." >&2
  exit 1
fi

[ -f resultados/calibracao.json ] || ./bancada.sh calibrar
./bancada.sh varredura -nome principal
./bancada.sh analise --varredura principal
echo "== campanha concluída"
