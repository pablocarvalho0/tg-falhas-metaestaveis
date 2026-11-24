# Reprodutibilidade: a mesma campanha em duas máquinas

**Por que isso importa.** Uma falha metaestável depende de margens pequenas: capacidade, tempo
de serviço, engasgadas do ambiente. Se a fronteira entre "recupera" e "fica preso" mudasse só
por trocar de máquina, os resultados descreveriam o laptop, e não as políticas. Por isso
rodamos a mesma campanha numa segunda máquina, bem diferente da primeira, sem mudar uma linha
do código nem os parâmetros do experimento.

> **Resultado em uma frase:** na repetição 1, **31 das 32 configurações tiveram o mesmo
> desfecho nas duas máquinas**. A única divergência foi o *momento* em que o limite fixo
> quebrou numa configuração de fronteira, e não *se* ele quebrou. Todas as conclusões do
> [`resultados.md`](resultados.md) se mantêm.

**Status dos dados**

| Repetição | Máquina A (Mac) | Máquina B (Linux) |
|---|---|---|
| 1 (32 execuções) | dados só no laptop; números no `resultados.md` | completa, em `resultados/principal/` |
| 2 e 3 (64 execuções) | — | 10 de 64 feitas; a campanha foi interrompida pelo limite de tempo da sessão |

## As duas máquinas

| | Máquina A | Máquina B |
|---|---|---|
| Hardware | MacBook, Apple Silicon (arm64) | VM em nuvem, Intel Xeon 2,3 GHz (x86_64), KVM |
| Ambiente Docker | Colima: VM Linux 6.8, **2 vCPU, 2 GB** | Linux 6.18, **4 vCPU, 15 GB**, Docker 29.10 |
| Data | 21/11/2026 | 24/11/2026 |

Código, imagens base (mesmas versões), parâmetros, sementes e ordem das execuções: idênticos.

### Diferenças operacionais na máquina B

Nenhuma delas muda o código nem o desenho experimental:

- **Imagens base:** vieram do espelho `mirror.gcr.io`, nas mesmas tags, porque o Docker Hub
  limitou as requisições (HTTP 429).
- **Toxiproxy:** a mesma versão, 2.12.0, compilada do código-fonte oficial. O download do
  `ghcr.io` é bloqueado pela rede.
- **Serviço e orquestrador:** compilados no host com Go 1.25, depois de `go vet` e dos 40 testes
  unitários passarem. Os containers de build não têm internet.
- **`nofile` = 20000** em vez de 65536, que é o teto do sandbox. Bastou: o k6 não descartou
  nenhuma iteração.
- **Interrupções:** a VM foi recolhida uma vez com a sessão ociosa, e a varredura foi retomada.
  Execuções sem `manifesto.json` são refeitas, como previsto em
  [`metodologia.md`](metodologia.md) §10.
- **Guardas de medição:** nenhuma execução foi descartada (na máquina A, 1 foi).

## Calibração

| | Máquina A | Máquina B | Diferença |
|---|---:|---:|---:|
| Capacidade μ | 190,45 req/s | **197,04 req/s** | +3,5 % |
| Tempo de serviço Ŝ | 42,03 ms | **40,59 ms** | −3,4 % |
| p95 do tempo de serviço | 44,8 ms | 44,8 ms | 0 |
| Desvio entre as 3 janelas (μ / Ŝ) | 0,11 % / 0,07 % | 0,11 % / 0,07 % | — |
| μ contado × C/Ŝ (lei de Little) | 0,14 % | 0,06 % | — |
| W = 150 − Ŝ − 10 | 98 ms | **99 ms** | +1 ms |

A máquina B é ~3,5 % mais rápida no tempo médio. O p95 é idêntico porque ele é dominado pela
consulta de 40 ms no Postgres. O orquestrador deriva Ŝ e W da calibração de cada máquina, e as
políticas se ajustam sozinhas.

## Desfechos, repetição 1: célula a célula

Mesma configuração e mesma semente nas duas máquinas. Os percentuais são a fração respondida
no prazo nos últimos 30 s (imp. = impacientes, pac. = pacientes).

| Fila | Cliente | Carga | Máquina A (Mac) | Máquina B (Linux) | |
|---|---|---:|---|---|---|
| Fila fixa | espera fixa | 20 % | recuperou, 0 s | recuperou, 0 s | ✅ |
| | | 50 % | presa, 0 % | presa, 0 %, amplif. 4,00 | ✅ |
| | | 70 % | presa, 0 % | presa, 0 %, amplif. 4,00 | ✅ |
| | | 90 % | caiu antes da falha | caiu antes da falha | ✅ |
| | backoff | 20 % | recuperou, 0 s | recuperou, 0 s | ✅ |
| | | 50 % | presa, 0,7 % | presa, 0 %, amplif. 4,01 | ✅ |
| | | 70 % | presa, 0 % | presa, 0 % | ✅ |
| | | 90 % | caiu antes da falha | caiu antes da falha | ✅ |
| Limite fixo | espera fixa | 20 % | recuperou, 0 s | recuperou, 0 s | ✅ |
| | | 50 % | recuperou, 1 s; 94,7 % (imp. 89,6 %) | recuperou, 1 s; 100 % (imp. 100 %) | ⚠️ intensidade |
| | | 70 % | **caiu antes da falha** (aos 13 s); imp. 3,4 %, pac. 76,3 % | **presa depois da falha**; imp. 0,3 %, pac. 69,1 % | ❌ momento |
| | | 90 % | caiu antes da falha; imp. 2,3 %, pac. 64,2 % | caiu antes da falha; imp. 1,0 %, pac. 61,1 % | ✅ |
| | backoff | 20 % | recuperou, 0 s | recuperou, 0 s | ✅ |
| | | 50 % | recuperou, 1 s | recuperou, 3 s | ✅ |
| | | 70 % | presa, 45 % (imp. 0,4 %, pac. 93,2 %) | presa, 44,4 % (imp. 0,2 %, pac. 91,1 %) | ✅ |
| | | 90 % | caiu antes da falha; imp. 0,1 %, pac. 82,5 % | caiu antes da falha; imp. 0,1 %, pac. 81,9 % | ✅ |
| Prazo por pedido | ambos | 20–90 % | recuperou nas 8, 0–1 s | recuperou nas 8, 0 s | ✅ |
| LIFO adaptativo | ambos | 20–90 % | recuperou nas 8, 0 s | recuperou nas 8, 0 s | ✅ |

**31 de 32 com o mesmo desfecho.** Os números de quem ficou preso são quase idênticos:

- amplificação de 4,00–4,04 nas duas máquinas;
- limite fixo com backoff a 70 %: 45 % no Mac, 44,4 % no Linux.

### As duas diferenças, explicadas

1. **Limite fixo, espera fixa, 70 %: caiu antes × depois da falha.** Nas duas máquinas, o
   estado final é o mesmo: impacientes presos (~0 %) e pacientes atendidos (~70 %). Muda só o
   gatilho.
   - **No Mac**, uma rajada normal durante o regime bastou para disparar o laço. É o estado
     *vulnerável* da metaestabilidade: qualquer empurrão derruba.
   - **No Linux**, com 3,5 % mais capacidade e o dobro de vCPUs (menos engasgadas), o regime
     aguentou e foi preciso a falha injetada.

   A configuração fica bem na fronteira: o resultado é "vai cair", e a máquina decide só quando.
2. **Limite fixo, espera fixa, 50 %: 89,6 % × 100 % dos impacientes.** O mesmo mecanismo: com
   Ŝ menor, sobra mais folga para quem só pode esperar 48 ms.

Essas duas células são as que o [`resultados.md`](resultados.md) já apontava como frágeis. O
limite fixo dimensionado pelo cliente médio deixa os impacientes na beira do laço. A segunda
máquina confirma isso de outro ângulo: **onde a política é robusta (prazo por pedido, LIFO), as
máquinas concordam; onde ela é frágil (limite fixo), a máquina mexe no resultado.**

## Custo quando nada está errado (regime, 90 % de carga)

| Política | Cliente | Descartes/s, A | Descartes/s, B | Desnecessários (pac.)/s, A | Desnecessários (pac.)/s, B | LIFO ligado no regime, A | LIFO ligado no regime, B |
|---|---|---:|---:|---:|---:|---:|---:|
| Prazo por pedido | espera fixa | 0,85 | 1,14 | 0 | 0 | — | — |
| Prazo por pedido | backoff | 3,05 | 4,15 | 0 | 0 | — | — |
| LIFO adaptativo | espera fixa | 3,07 | 6,05 | 1,55 | 2,76 | 6,7 % | 23,3 % |
| LIFO adaptativo | backoff | 12,39 | 6,89 | 6,34 | 3,02 | 16,7 % | 20,0 % |

Os valores absolutos variam (são poucos eventos por segundo e uma execução por célula), mas a
conclusão é a mesma nas duas máquinas:

- o **prazo por pedido nunca descarta um pedido que ainda daria tempo**;
- o **LIFO** a 90 % liga o detector em rajadas normais e descarta pacientes que seriam atendidos.

## Conclusão

1. **O resultado principal não depende da máquina.**
   - A fila fixa fica presa a partir de 50 % nas duas máquinas.
   - Prazo por pedido e LIFO recuperam nas 16 configurações de cada máquina.
   - Backoff não muda nenhum desfecho em nenhuma das duas.
2. **A calibração por máquina funciona.** μ e Ŝ diferem 3,5 %, mas, como W e Ŝ são derivados da
   calibração, as políticas se comportam igual.
3. **A máquina só pesa nas configurações de fronteira:** o limite fixo, que já era a política
   frágil, muda de *quando* quebra, não de *se* quebra.
4. **Lição de método:** reproduzir em outra máquina separa o que é propriedade da política do
   que é propriedade do ambiente. Vale incluir isso no protocolo de qualquer experimento de
   metaestabilidade.

**Pendente:** repetições 2 e 3 completas nas duas máquinas, para ter média ± desvio por célula.
A estatística das execuções já feitas está em `resultados/estatistica_principal.md`.
