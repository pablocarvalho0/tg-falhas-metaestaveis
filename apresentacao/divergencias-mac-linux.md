# O que muda nos slides: resultados do Linux (máquina B) × Mac (máquina A)

Os slides atuais usam os números do **Mac**. Abaixo, o que trocar para a versão **Linux**, slide
por slide. Fonte: `experimento/resultados/` (repetição 1 completa, 32 execuções). Análise
completa: [`experimento/reprodutibilidade.md`](../experimento/reprodutibilidade.md).

**Resumo:**

- **31 de 32 configurações têm o mesmo desfecho.** As conclusões não mudam.
- **A divergência real é uma só:** o limite fixo com espera fixa a 70 % caiu *antes* da falha
  no Mac e *depois* dela no Linux.
- **O resto são números de calibração e intensidades.**

As figuras novas, já geradas a partir dos dados do Linux, estão em
`experimento/resultados/figuras/principal_carga{20,50,70,90}.png`. É só trocar as imagens dos
slides 14 a 17.

---

## Slide 10 · Metodologia 1 de 2

Nada muda.

## Slide 11 · Metodologia 2 de 2: "Medido, não suposto"

| Onde | Mac (atual) | Linux (novo) |
|---|---|---|
| Capacidade | μ = 190,45 req/s | μ = **197,04 req/s** |
| Tempo de serviço | Ŝ = 42,03 ms | Ŝ = **40,59 ms** |
| Concordância das 3 medições | 0,07 % | 0,07 % (igual) |
| Ŝ explica μ | 0,14 % | **0,06 %** |
| W | 98 ms | **99 ms** |

## Slide 12 · Protótipo

| Onde | Mac | Linux |
|---|---|---|
| "lei de Little explica μ em 0,14 %" | 0,14 % | **0,06 %** |
| Guardas | 1 execução descartada | **0 descartadas** (pode acrescentar) |

## Slide 13 · Visão geral dos resultados

A tabela fica **igual**. Sugestão: trocar o subtítulo para "32 execuções, em duas máquinas" e
acrescentar um bullet:

> Rodamos tudo de novo numa segunda máquina (Linux, 4 vCPU): **31 de 32 desfechos iguais**.

## Slide 14 · 20 %

O texto fica igual: as 8 recuperam em 0 s. Trocar a figura por `principal_carga20.png`.

## Slide 15 · 50 %

| Onde | Mac | Linux |
|---|---|---|
| Fila fixa, desperdício | ~190 req/s | **~197 req/s** |
| Fila fixa, atendidos | 0–0,7 % | **0 %** (os dois clientes) |
| Limite fixo | "recupera em 1 s, mas com espera fixa os impacientes terminam em 90 %" | **recupera em 1–3 s; impacientes terminam em 100 %** |

Trocar a figura por `principal_carga50.png`.

**Atenção:** no Linux, o limite fixo a 50 % não mostra mais o prejuízo dos impacientes. O
argumento "limite fixo prejudica o impaciente" passa a se apoiar só em 70 e 90 %, onde ele
continua claríssimo.

## Slide 16 · 70 %: a única divergência de desfecho

| Onde | Mac | Linux |
|---|---|---|
| Impacientes atendidos | 0–3 % | **0,2–0,3 %** |
| Desperdício | ~135 req/s | **~135–150 req/s** |
| "Com espera fixa, cai aos 13 s, sem falha." | caiu **antes** da falha | ficou presa **depois** da falha |

Sugestão de frase: *"Com espera fixa, no Mac cai antes mesmo da falha; no Linux, mais rápido,
aguenta até a falha e trava depois. Mesmo destino, gatilho diferente."*

Trocar a figura por `principal_carga70.png`.

## Slide 17 · 90 %

| Onde | Mac | Linux |
|---|---|---|
| Prazo: recuperação | 0–1 s | **0 s** |
| Prazo: descartes no regime | ~3 descartes/s | **~1–4 descartes/s**, nenhum desnecessário (igual) |
| LIFO: detector em rajadas normais | até 17 % do tempo | **até 23 %** |
| LIFO: pedidos úteis descartados | até 6 req/s | **até 3 req/s** |

Trocar a figura por `principal_carga90.png`.

## Slide 18 · Análise matemática

| Onde | Mac | Linux |
|---|---|---|
| Capacidade | 190,5 req/s (medido 190,45) | C/Ŝ = 8/40,6 ms = **197 req/s (medido 197,04)** |
| Capacidade na falha | 33 req/s | 8/240,6 ms = **33 req/s** (igual) |
| Espera com fila cheia | 336 ms | 64/197 = **325 ms** (continua > timeouts) |
| Amplificação presa | 4 (medido 4,00) | 4 (medido **4,00–4,02**) |
| W > espera útil do impaciente | 98 > 48 ms | **99 > 49 ms** |

## Slide 19 · Modelo × bancada

Nada muda: o simulador usa os parâmetros do Mac, e os desfechos são os mesmos.

## Slide 21 · Limitações

Trocar "Um laptop, 2 vCPUs" por **"Duas máquinas (laptop arm64 com 2 vCPU e VM x86 com 4 vCPU);
desfechos iguais em 31 de 32"**.

## Slide novo sugerido (depois do 17): "Reprodutibilidade"

> **A mesma campanha em duas máquinas**
>
> | | Mac (arm64, 2 vCPU) | Linux (x86, 4 vCPU) |
> |---|---|---|
> | μ | 190,45 req/s | 197,04 req/s |
> | Desfechos iguais | — | 31 de 32 |
>
> - Prazo por pedido e LIFO: recuperam nas 16 configurações das duas máquinas.
> - A única diferença foi o limite fixo a 70 %: cai antes da falha no Mac e depois dela no
>   Linux.
> - **Onde a política é robusta, as máquinas concordam; onde ela é frágil, a máquina decide
>   quando ela quebra.**
