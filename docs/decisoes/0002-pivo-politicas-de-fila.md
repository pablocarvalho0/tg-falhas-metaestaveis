# 0002 — Mudança de foco: das políticas de retry para as políticas de fila

**Status:** aceita · **Data:** 21/11/2026 · Substitui parcialmente a 0001 (seção Experimento)

## Contexto

A 0001 perguntava: *para quais cargas e políticas de retry o goodput não volta depois de um
atraso transitório no banco?* O experimento 1 respondeu:

- com retry, o serviço de fila fixa fica preso depois da falha a partir de ~1/(N+1) da capacidade;
- **backoff exponencial com jitter não resolve**: com teto de 1 s, ele espaça os retries mas
  não reduz quantos são, e o servidor continua saturado;
- cancelar pedidos abandonados na saída da fila também não resolveu na configuração testada.

Com isso, variar só o cliente deixou de ser uma pergunta interessante: a resposta já era
"nenhuma política de retry salva".

## Decisão

O experimento 2 move o foco para o **servidor**: qual política de fila quebra o laço?

| Fator | Níveis |
|---|---|
| Política de fila | fila fixa · limite de tempo fixo · prazo por pedido · LIFO adaptativo |
| Retry no cliente | espera fixa · backoff + jitter |
| Carga | 20 · 50 · 70 · 90 % de μ |
| Repetições | 3 → 96 execuções |

Duas classes de cliente (timeouts de 100 e 200 ms) para que "limite fixo" e "prazo por pedido"
não sejam idênticos por construção. Detalhes: `experimento/plano.md` e `experimento/metodologia.md`.

## Consequências

- O que muda em relação à proposta de 27/out (precisa ser explicado na apresentação final):
  - as 3 políticas de retry (nenhuma, imediata, backoff) viram 2 (espera fixa, backoff);
  - as cargas de 60–90 % viram 20–90 %;
  - entram 4 políticas de fila no servidor, que não estavam na proposta.
- A bancada (k6 → serviço Go → Toxiproxy → Postgres, orquestrador Go, Prometheus) é a mesma.
- Load shedding / políticas de fila saem de "fica para o TG" e entram no exame. Retry budget e
  circuit breaker continuam no TG.
- Experimento 1 precisa estar no repositório (`exame_1/`) porque o relatório se apoia nele.
