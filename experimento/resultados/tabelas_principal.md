# Tabelas da varredura `principal`

Geradas por analise.py; não edite.

## Desfecho

| Fila | Cliente | Carga | Execuções | Caíram antes do atraso | Recuperaram | Tempo de recuperação | Atendidos no prazo ao final | Amplif. final | Desperdício final (req/s) | p99 final no serviço (ms) |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 · Fila fixa | Espera fixa | 20 % | 1 | 0 | 1 de 1 | 0 s | 100,1 % | 1,00 | 0,0 | 45 |
| 1 · Fila fixa | Espera fixa | 50 % | 1 | 0 | 0 de 1 | não | 0,0 % | 4,00 | 196,6 | 399 |
| 1 · Fila fixa | Espera fixa | 70 % | 1 | 0 | 0 de 1 | não | 0,0 % | 4,00 | 196,5 | 399 |
| 1 · Fila fixa | Espera fixa | 90 % | 1 | 1 | 0 de 0 | não | — | — | — | — |
| 1 · Fila fixa | Backoff + jitter | 20 % | 1 | 0 | 1 de 1 | 0 s | 100,1 % | 1,00 | 0,0 | 45 |
| 1 · Fila fixa | Backoff + jitter | 50 % | 1 | 0 | 0 de 1 | não | 0,0 % | 4,01 | 197,1 | 399 |
| 1 · Fila fixa | Backoff + jitter | 70 % | 1 | 0 | 0 de 1 | não | 0,0 % | 4,01 | 196,6 | 399 |
| 1 · Fila fixa | Backoff + jitter | 90 % | 1 | 1 | 0 de 0 | não | — | — | — | — |
| 2 · Tempo fixo (W) | Espera fixa | 20 % | 1 | 0 | 1 de 1 | 0 s | 100,2 % | 1,00 | 0,0 | 45 |
| 2 · Tempo fixo (W) | Espera fixa | 50 % | 1 | 0 | 1 de 1 | 1 s | 100,0 % | 1,00 | 0,0 | 54 |
| 2 · Tempo fixo (W) | Espera fixa | 70 % | 1 | 0 | 0 de 1 | não | 33,7 % | 3,15 | 149,8 | 150 |
| 2 · Tempo fixo (W) | Espera fixa | 90 % | 1 | 1 | 0 de 0 | não | — | — | — | — |
| 2 · Tempo fixo (W) | Backoff + jitter | 20 % | 1 | 0 | 1 de 1 | 0 s | 100,2 % | 1,00 | 0,0 | 45 |
| 2 · Tempo fixo (W) | Backoff + jitter | 50 % | 1 | 0 | 1 de 1 | 3 s | 100,1 % | 1,00 | 0,0 | 53 |
| 2 · Tempo fixo (W) | Backoff + jitter | 70 % | 1 | 0 | 0 de 1 | não | 44,4 % | 3,05 | 134,4 | 150 |
| 2 · Tempo fixo (W) | Backoff + jitter | 90 % | 1 | 1 | 0 de 0 | não | — | — | — | — |
| 3 · Prazo por pedido | Espera fixa | 20 % | 1 | 0 | 1 de 1 | 0 s | 100,1 % | 1,00 | 0,0 | 45 |
| 3 · Prazo por pedido | Espera fixa | 50 % | 1 | 0 | 1 de 1 | 0 s | 100,0 % | 1,00 | 0,0 | 53 |
| 3 · Prazo por pedido | Espera fixa | 70 % | 1 | 0 | 1 de 1 | 0 s | 100,0 % | 1,00 | 0,0 | 69 |
| 3 · Prazo por pedido | Espera fixa | 90 % | 1 | 0 | 1 de 1 | 0 s | 99,9 % | 1,02 | 0,0 | 98 |
| 3 · Prazo por pedido | Backoff + jitter | 20 % | 1 | 0 | 1 de 1 | 0 s | 100,1 % | 1,00 | 0,0 | 45 |
| 3 · Prazo por pedido | Backoff + jitter | 50 % | 1 | 0 | 1 de 1 | 0 s | 100,0 % | 1,00 | 0,0 | 54 |
| 3 · Prazo por pedido | Backoff + jitter | 70 % | 1 | 0 | 1 de 1 | 0 s | 100,0 % | 1,00 | 0,0 | 70 |
| 3 · Prazo por pedido | Backoff + jitter | 90 % | 1 | 0 | 1 de 1 | 0 s | 100,0 % | 1,01 | 0,0 | 98 |
| 4 · LIFO adaptativo | Espera fixa | 20 % | 1 | 0 | 1 de 1 | 0 s | 100,1 % | 1,00 | 0,0 | 45 |
| 4 · LIFO adaptativo | Espera fixa | 50 % | 1 | 0 | 1 de 1 | 0 s | 100,0 % | 1,00 | 0,0 | 53 |
| 4 · LIFO adaptativo | Espera fixa | 70 % | 1 | 0 | 1 de 1 | 0 s | 99,9 % | 1,00 | 0,2 | 68 |
| 4 · LIFO adaptativo | Espera fixa | 90 % | 1 | 0 | 1 de 1 | 0 s | 99,9 % | 1,04 | 1,1 | 106 |
| 4 · LIFO adaptativo | Backoff + jitter | 20 % | 1 | 0 | 1 de 1 | 0 s | 100,1 % | 1,00 | 0,0 | 45 |
| 4 · LIFO adaptativo | Backoff + jitter | 50 % | 1 | 0 | 1 de 1 | 0 s | 100,0 % | 1,00 | 0,0 | 54 |
| 4 · LIFO adaptativo | Backoff + jitter | 70 % | 1 | 0 | 1 de 1 | 0 s | 100,1 % | 1,00 | 0,1 | 67 |
| 4 · LIFO adaptativo | Backoff + jitter | 90 % | 1 | 0 | 1 de 1 | 0 s | 100,0 % | 1,04 | 0,8 | 100 |

## Por classe de cliente

| Fila | Cliente | Carga | Atendidos ao final, impatient | Atendidos ao final, patient | Recuperação impatient | Recuperação patient | Desperdício impatient (req/s) | Desperdício patient (req/s) | Latência ok impatient (ms) | Latência ok patient (ms) |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 · Fila fixa | Espera fixa | 20 % | 100,1 % | 100,1 % | 1 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 1 · Fila fixa | Espera fixa | 50 % | 0,0 % | 0,0 % | não | não | 95,5 | 101,2 | — | — |
| 1 · Fila fixa | Espera fixa | 70 % | 0,0 % | 0,0 % | não | não | 103,6 | 93,0 | — | — |
| 1 · Fila fixa | Espera fixa | 90 % | — | — | não | não | — | — | — | — |
| 1 · Fila fixa | Backoff + jitter | 20 % | 100,0 % | 100,2 % | 1 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 1 · Fila fixa | Backoff + jitter | 50 % | 0,0 % | 0,0 % | não | não | 101,2 | 95,9 | — | — |
| 1 · Fila fixa | Backoff + jitter | 70 % | 0,0 % | 0,0 % | não | não | 100,0 | 96,6 | — | — |
| 1 · Fila fixa | Backoff + jitter | 90 % | — | — | não | não | — | — | — | — |
| 2 · Tempo fixo (W) | Espera fixa | 20 % | 100,1 % | 100,3 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 2 · Tempo fixo (W) | Espera fixa | 50 % | 100,0 % | 100,0 % | 2 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 2 · Tempo fixo (W) | Espera fixa | 70 % | 0,3 % | 69,1 % | não | não | 149,8 | 0,0 | 89 | 126 |
| 2 · Tempo fixo (W) | Espera fixa | 90 % | — | — | não | não | — | — | — | — |
| 2 · Tempo fixo (W) | Backoff + jitter | 20 % | 100,3 % | 100,1 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 2 · Tempo fixo (W) | Backoff + jitter | 50 % | 100,0 % | 100,1 % | 3 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 2 · Tempo fixo (W) | Backoff + jitter | 70 % | 0,2 % | 91,1 % | não | 47 s | 134,4 | 0,0 | 98 | 130 |
| 2 · Tempo fixo (W) | Backoff + jitter | 90 % | — | — | não | não | — | — | — | — |
| 3 · Prazo por pedido | Espera fixa | 20 % | 100,0 % | 100,2 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 3 · Prazo por pedido | Espera fixa | 50 % | 100,0 % | 100,0 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 3 · Prazo por pedido | Espera fixa | 70 % | 100,0 % | 100,0 % | 0 s | 0 s | 0,0 | 0,0 | 43 | 43 |
| 3 · Prazo por pedido | Espera fixa | 90 % | 99,9 % | 99,9 % | 0 s | 0 s | 0,0 | 0,0 | 54 | 54 |
| 3 · Prazo por pedido | Backoff + jitter | 20 % | 100,0 % | 100,1 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 3 · Prazo por pedido | Backoff + jitter | 50 % | 100,1 % | 100,0 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 3 · Prazo por pedido | Backoff + jitter | 70 % | 100,0 % | 99,9 % | 0 s | 0 s | 0,0 | 0,0 | 43 | 43 |
| 3 · Prazo por pedido | Backoff + jitter | 90 % | 100,0 % | 100,0 % | 1 s | 0 s | 0,0 | 0,0 | 54 | 54 |
| 4 · LIFO adaptativo | Espera fixa | 20 % | 100,1 % | 100,1 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 4 · LIFO adaptativo | Espera fixa | 50 % | 100,1 % | 100,0 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 4 · LIFO adaptativo | Espera fixa | 70 % | 100,0 % | 99,9 % | 0 s | 0 s | 0,2 | 0,0 | 43 | 43 |
| 4 · LIFO adaptativo | Espera fixa | 90 % | 99,9 % | 99,9 % | 0 s | 1 s | 1,1 | 0,0 | 50 | 51 |
| 4 · LIFO adaptativo | Backoff + jitter | 20 % | 100,0 % | 100,3 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 4 · LIFO adaptativo | Backoff + jitter | 50 % | 100,0 % | 100,0 % | 0 s | 0 s | 0,0 | 0,0 | 41 | 41 |
| 4 · LIFO adaptativo | Backoff + jitter | 70 % | 100,0 % | 100,1 % | 0 s | 0 s | 0,1 | 0,0 | 43 | 43 |
| 4 · LIFO adaptativo | Backoff + jitter | 90 % | 100,0 % | 100,1 % | 0 s | 0 s | 0,8 | 0,0 | 50 | 51 |

## Custo e detector de sobrecarga

| Fila | Cliente | Carga | Atendidos no regime, impatient | Atendidos no regime, patient | Descartes no regime (req/s) | Desnecessários no regime, impatient | Desnecessários no regime, patient | Desnecessários na recuperação, impatient | Desnecessários na recuperação, patient | LIFO ligado no regime | LIFO ligado no atraso | LIFO ligado ao final |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 · Fila fixa | Espera fixa | 20 % | 100,00 % | 99,80 % | 0,00 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 1 · Fila fixa | Espera fixa | 50 % | 100,00 % | 99,90 % | 1,11 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 1 · Fila fixa | Espera fixa | 70 % | 100,00 % | 100,10 % | 0,60 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 1 · Fila fixa | Espera fixa | 90 % | — | — | — | — | — | — | — | — | — | — |
| 1 · Fila fixa | Backoff + jitter | 20 % | 100,00 % | 99,90 % | 0,00 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 1 · Fila fixa | Backoff + jitter | 50 % | 100,00 % | 99,90 % | 0,79 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 1 · Fila fixa | Backoff + jitter | 70 % | 100,00 % | 100,10 % | 0,00 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 1 · Fila fixa | Backoff + jitter | 90 % | — | — | — | — | — | — | — | — | — | — |
| 2 · Tempo fixo (W) | Espera fixa | 20 % | 100,10 % | 99,90 % | 0,08 | 0,00 | 0,06 | 0,00 | 0,02 | 0,0 % | 0 % | 0,0 % |
| 2 · Tempo fixo (W) | Espera fixa | 50 % | 100,20 % | 99,90 % | 0,26 | 0,00 | 0,01 | 0,00 | 0,77 | 0,0 % | 0 % | 0,0 % |
| 2 · Tempo fixo (W) | Espera fixa | 70 % | 100,00 % | 100,10 % | 1,48 | 0,00 | 0,05 | 0,00 | 106,14 | 0,0 % | 0 % | 0,0 % |
| 2 · Tempo fixo (W) | Espera fixa | 90 % | — | — | — | — | — | — | — | — | — | — |
| 2 · Tempo fixo (W) | Backoff + jitter | 20 % | 100,00 % | 99,90 % | 0,30 | 0,00 | 0,14 | 0,00 | 0,09 | 0,0 % | 0 % | 0,0 % |
| 2 · Tempo fixo (W) | Backoff + jitter | 50 % | 99,90 % | 100,00 % | 0,18 | 0,00 | 0,02 | 0,00 | 0,62 | 0,0 % | 0 % | 0,0 % |
| 2 · Tempo fixo (W) | Backoff + jitter | 70 % | 100,00 % | 100,10 % | 1,52 | 0,00 | 0,10 | 0,00 | 71,61 | 0,0 % | 0 % | 0,0 % |
| 2 · Tempo fixo (W) | Backoff + jitter | 90 % | — | — | — | — | — | — | — | — | — | — |
| 3 · Prazo por pedido | Espera fixa | 20 % | 100,10 % | 99,90 % | 0,18 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 3 · Prazo por pedido | Espera fixa | 50 % | 100,20 % | 100,00 % | 0,27 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 3 · Prazo por pedido | Espera fixa | 70 % | 99,90 % | 100,00 % | 2,15 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 3 · Prazo por pedido | Espera fixa | 90 % | 99,90 % | 100,10 % | 1,14 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 3 · Prazo por pedido | Backoff + jitter | 20 % | 100,00 % | 100,10 % | 0,07 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 3 · Prazo por pedido | Backoff + jitter | 50 % | 100,10 % | 100,10 % | 0,00 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 3 · Prazo por pedido | Backoff + jitter | 70 % | 100,10 % | 100,00 % | 0,28 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 3 · Prazo por pedido | Backoff + jitter | 90 % | 99,90 % | 100,10 % | 4,15 | 0,00 | 0,00 | 0,00 | 0,00 | 0,0 % | 0 % | 0,0 % |
| 4 · LIFO adaptativo | Espera fixa | 20 % | 100,10 % | 99,90 % | 0,39 | 0,00 | 0,11 | 0,00 | 0,05 | 0,0 % | 100 % | 0,0 % |
| 4 · LIFO adaptativo | Espera fixa | 50 % | 100,20 % | 99,90 % | 0,47 | 0,00 | 0,09 | 0,00 | 0,44 | 0,0 % | 100 % | 0,0 % |
| 4 · LIFO adaptativo | Espera fixa | 70 % | 99,80 % | 100,10 % | 0,88 | 0,00 | 0,34 | 0,00 | 0,95 | 0,0 % | 100 % | 0,0 % |
| 4 · LIFO adaptativo | Espera fixa | 90 % | 100,00 % | 100,10 % | 6,05 | 0,00 | 2,76 | 0,00 | 5,36 | 23,3 % | 100 % | 16,7 % |
| 4 · LIFO adaptativo | Backoff + jitter | 20 % | 100,00 % | 99,90 % | 0,34 | 0,00 | 0,07 | 0,00 | 0,10 | 0,0 % | 100 % | 0,0 % |
| 4 · LIFO adaptativo | Backoff + jitter | 50 % | 99,90 % | 99,90 % | 0,75 | 0,00 | 0,16 | 0,00 | 0,48 | 0,0 % | 100 % | 0,0 % |
| 4 · LIFO adaptativo | Backoff + jitter | 70 % | 100,00 % | 100,10 % | 1,17 | 0,00 | 0,19 | 0,00 | 1,28 | 3,3 % | 100 % | 0,0 % |
| 4 · LIFO adaptativo | Backoff + jitter | 90 % | 100,00 % | 100,00 % | 6,89 | 0,00 | 3,02 | 0,00 | 5,85 | 20,0 % | 100 % | 10,0 % |

## Descartes por motivo (total por execução, média)

| Fila | Cliente | Carga | full | evicted | expired | deadline | stale |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| 1 · Fila fixa | Espera fixa | 20 % | 2413 | 0 | 0 | 0 | 0 |
| 1 · Fila fixa | Espera fixa | 50 % | 24846 | 0 | 0 | 0 | 0 |
| 1 · Fila fixa | Espera fixa | 70 % | 42156 | 0 | 0 | 0 | 0 |
| 1 · Fila fixa | Espera fixa | 90 % | 66729 | 0 | 0 | 0 | 0 |
| 1 · Fila fixa | Backoff + jitter | 20 % | 2403 | 0 | 0 | 0 | 0 |
| 1 · Fila fixa | Backoff + jitter | 50 % | 24787 | 0 | 0 | 0 | 0 |
| 1 · Fila fixa | Backoff + jitter | 70 % | 42112 | 0 | 0 | 0 | 0 |
| 1 · Fila fixa | Backoff + jitter | 90 % | 66648 | 0 | 0 | 0 | 0 |
| 2 · Tempo fixo (W) | Espera fixa | 20 % | 0 | 0 | 2438 | 0 | 0 |
| 2 · Tempo fixo (W) | Espera fixa | 50 % | 2295 | 0 | 4957 | 0 | 0 |
| 2 · Tempo fixo (W) | Espera fixa | 70 % | 5174 | 0 | 26343 | 0 | 0 |
| 2 · Tempo fixo (W) | Espera fixa | 90 % | 13998 | 0 | 38278 | 0 | 0 |
| 2 · Tempo fixo (W) | Backoff + jitter | 20 % | 0 | 0 | 2434 | 0 | 0 |
| 2 · Tempo fixo (W) | Backoff + jitter | 50 % | 2281 | 0 | 5002 | 0 | 0 |
| 2 · Tempo fixo (W) | Backoff + jitter | 70 % | 5088 | 0 | 24770 | 0 | 0 |
| 2 · Tempo fixo (W) | Backoff + jitter | 90 % | 13659 | 0 | 37569 | 0 | 0 |
| 3 · Prazo por pedido | Espera fixa | 20 % | 0 | 0 | 0 | 2439 | 0 |
| 3 · Prazo por pedido | Espera fixa | 50 % | 2509 | 0 | 0 | 4631 | 0 |
| 3 · Prazo por pedido | Espera fixa | 70 % | 5216 | 0 | 0 | 5132 | 0 |
| 3 · Prazo por pedido | Espera fixa | 90 % | 8190 | 0 | 0 | 5607 | 0 |
| 3 · Prazo por pedido | Backoff + jitter | 20 % | 0 | 0 | 0 | 2435 | 0 |
| 3 · Prazo por pedido | Backoff + jitter | 50 % | 2599 | 0 | 0 | 4539 | 0 |
| 3 · Prazo por pedido | Backoff + jitter | 70 % | 5173 | 0 | 0 | 5157 | 0 |
| 3 · Prazo por pedido | Backoff + jitter | 90 % | 8228 | 0 | 0 | 5594 | 0 |
| 4 · LIFO adaptativo | Espera fixa | 20 % | 0 | 0 | 0 | 0 | 2442 |
| 4 · LIFO adaptativo | Espera fixa | 50 % | 0 | 3289 | 0 | 0 | 3850 |
| 4 · LIFO adaptativo | Espera fixa | 70 % | 0 | 6662 | 1 | 0 | 3687 |
| 4 · LIFO adaptativo | Espera fixa | 90 % | 0 | 12740 | 18 | 0 | 1570 |
| 4 · LIFO adaptativo | Backoff + jitter | 20 % | 0 | 0 | 0 | 0 | 2437 |
| 4 · LIFO adaptativo | Backoff + jitter | 50 % | 0 | 2743 | 0 | 0 | 4391 |
| 4 · LIFO adaptativo | Backoff + jitter | 70 % | 0 | 7829 | 2 | 0 | 2523 |
| 4 · LIFO adaptativo | Backoff + jitter | 90 % | 0 | 12509 | 19 | 0 | 1937 |

## Qualidade da medição

| Checagem | Valor |
| --- | ---: |
| Execuções | 32 |
| Iterações descartadas pelo k6 (soma) | 0 |
| Erros do cliente que não são timeout nem 503 (soma) | 0 |
| Erros do serviço ao falar com o banco (soma) | 0 |
| Erro da lei de Little no regime (mediana / máximo) | 2,48 % / 8,57 % |
| Maior congelamento do ambiente (mediana / máximo) | 18,2 ms / 45,0 ms |
