| Fila | Cliente | Carga | n | Recuperou | Preso/parcial | Caiu antes | Recuperação (s) | Respondidos ao fim (%) |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| 1 · Fila fixa | backoff | 20 % | 1 | 1 | 0 | 0 | 0.0 | 100.1 |
| 1 · Fila fixa | backoff | 50 % | 1 | 0 | 1 | 0 | — | 0.0 |
| 1 · Fila fixa | backoff | 70 % | 1 | 0 | 1 | 0 | — | 0.0 |
| 1 · Fila fixa | backoff | 90 % | 1 | 0 | 0 | 1 | — | 0.0 |
| 1 · Fila fixa | fixed | 20 % | 1 | 1 | 0 | 0 | 0.0 | 100.1 |
| 1 · Fila fixa | fixed | 50 % | 2 | 0 | 2 | 0 | — | 0.0 ± 0.0 |
| 1 · Fila fixa | fixed | 70 % | 2 | 0 | 2 | 0 | — | 0.0 ± 0.0 |
| 1 · Fila fixa | fixed | 90 % | 1 | 0 | 0 | 1 | — | 0.0 |
| 2 · Limite fixo | backoff | 20 % | 2 | 2 | 0 | 0 | 0.0 ± 0.0 | 100.1 ± 0.2 |
| 2 · Limite fixo | backoff | 50 % | 1 | 1 | 0 | 0 | 3.0 | 100.1 |
| 2 · Limite fixo | backoff | 70 % | 1 | 0 | 1 | 0 | — | 44.4 |
| 2 · Limite fixo | backoff | 90 % | 1 | 0 | 0 | 1 | — | 41.6 |
| 2 · Limite fixo | fixed | 20 % | 1 | 1 | 0 | 0 | 0.0 | 100.2 |
| 2 · Limite fixo | fixed | 50 % | 1 | 1 | 0 | 0 | 1.0 | 100.0 |
| 2 · Limite fixo | fixed | 70 % | 1 | 0 | 1 | 0 | — | 33.7 |
| 2 · Limite fixo | fixed | 90 % | 1 | 0 | 0 | 1 | — | 31.5 |
| 3 · Prazo por pedido | backoff | 20 % | 1 | 1 | 0 | 0 | 0.0 | 100.1 |
| 3 · Prazo por pedido | backoff | 50 % | 2 | 2 | 0 | 0 | 0.0 ± 0.0 | 100.0 ± 0.0 |
| 3 · Prazo por pedido | backoff | 70 % | 1 | 1 | 0 | 0 | 0.0 | 100.0 |
| 3 · Prazo por pedido | backoff | 90 % | 1 | 1 | 0 | 0 | 0.0 | 100.0 |
| 3 · Prazo por pedido | fixed | 20 % | 2 | 2 | 0 | 0 | 0.0 ± 0.0 | 100.1 ± 0.0 |
| 3 · Prazo por pedido | fixed | 50 % | 1 | 1 | 0 | 0 | 0.0 | 100.0 |
| 3 · Prazo por pedido | fixed | 70 % | 1 | 1 | 0 | 0 | 0.0 | 100.0 |
| 3 · Prazo por pedido | fixed | 90 % | 1 | 1 | 0 | 0 | 0.0 | 99.9 |
| 4 · LIFO adaptativo | backoff | 20 % | 2 | 2 | 0 | 0 | 0.0 ± 0.0 | 100.0 ± 0.1 |
| 4 · LIFO adaptativo | backoff | 50 % | 1 | 1 | 0 | 0 | 0.0 | 100.0 |
| 4 · LIFO adaptativo | backoff | 70 % | 1 | 1 | 0 | 0 | 0.0 | 100.1 |
| 4 · LIFO adaptativo | backoff | 90 % | 1 | 1 | 0 | 0 | 0.0 | 100.0 |
| 4 · LIFO adaptativo | fixed | 20 % | 2 | 2 | 0 | 0 | 0.0 ± 0.0 | 100.0 ± 0.1 |
| 4 · LIFO adaptativo | fixed | 50 % | 2 | 2 | 0 | 0 | 0.0 ± 0.0 | 100.0 ± 0.0 |
| 4 · LIFO adaptativo | fixed | 70 % | 2 | 2 | 0 | 0 | 0.0 ± 0.0 | 100.0 ± 0.1 |
| 4 · LIFO adaptativo | fixed | 90 % | 2 | 2 | 0 | 0 | 0.0 ± 0.0 | 100.0 ± 0.1 |
