#!/usr/bin/env bash
# Experimento 2 — políticas de fila contra o laço de retry (ver plano.md).
# Tudo roda em containers; no host só é preciso Docker com Compose.
#
#   ./bancada.sh tudo                      calibra μ e roda a varredura do plano (96 execuções, ~4 h 20)
#   ./bancada.sh calibrar                  só a calibração de μ
#   ./bancada.sh cenario -fila lifo -politica fixed -carga 70
#   ./bancada.sh varredura -nome piloto -filas fifo,lifo -politicas fixed -cargas 70 -reps 1
#   ./bancada.sh reprocessar -nome principal   refaz CSV e resumos a partir do Prometheus
#   ./bancada.sh analise                   gera figuras e tabelas em resultados/
#   ./bancada.sh testes                    roda os testes unitários (serviço e orquestrador)
#   ./bancada.sh subir | derrubar | estado
#
# Resultados: ./resultados (CSV de 1 s por execução, manifesto JSON, resumo.csv por varredura).
set -euo pipefail
cd "$(dirname "$0")"

if docker compose version >/dev/null 2>&1; then
  dc=(docker compose)
else
  dc=(docker-compose)
fi

subir() {
  "${dc[@]}" up -d --build --wait postgres toxiproxy servico prometheus
}

cmd="${1:-}"
[ $# -gt 0 ] && shift
case "$cmd" in
  subir)    subir ;;
  derrubar) "${dc[@]}" --profile execucao --profile analise down -v --remove-orphans ;;
  estado)   "${dc[@]}" ps ;;
  testes)
    # Os testes rodam dentro do build das imagens (go vet + go test).
    "${dc[@]}" --profile execucao build --no-cache servico bancada
    ;;
  analise)
    mkdir -p resultados
    "${dc[@]}" --profile analise run --rm --build analise "$@"
    ;;
  calibrar|cenario|varredura|tudo|reprocessar)
    mkdir -p resultados
    subir
    "${dc[@]}" --profile execucao run --rm --build bancada "$cmd" "$@"
    ;;
  *)
    sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
    ;;
esac
