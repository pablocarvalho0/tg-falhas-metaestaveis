# Reprodutibilidade: a mesma campanha em duas máquinas

**Por que isso importa.** Uma falha metaestável depende de margens pequenas: capacidade, tempo
de serviço, engasgadas do ambiente. Se a fronteira entre "recupera" e "fica preso" mudasse só
por trocar de máquina, os resultados descreveriam o laptop, e não as políticas. Por isso
rodamos a mesma campanha numa segunda máquina, bem diferente da primeira, sem mudar uma linha
do código nem os parâmetros do experimento.

> **Status:** campanha na máquina B **em andamento**. Esta página é atualizada ao fim das
> 96 execuções. Os dados brutos estão em `resultados/` (máquina B). Os dados da máquina A
> ficaram no laptop: só os números do `results.md` estão no repositório.

## As duas máquinas

| | Máquina A (results.md, slides) | Máquina B (esta página) |
|---|---|---|
| Hardware | MacBook, Apple Silicon (arm64) | VM em nuvem, Intel Xeon 2,3 GHz (x86_64), KVM |
| Ambiente Docker | Colima: VM Linux 6.8, **2 vCPU, 2 GB** | Linux 6.18, **4 vCPU, 15 GB**, Docker 29.10 |
| Energia / suspensão | na tomada, `caffeinate` | servidor; a VM é recolhida se a sessão fica ociosa |
| Data | 21/11/2026 | 24/11/2026 |

Código, imagens base (mesmas versões), parâmetros, sementes e ordem das execuções: idênticos.

### Diferenças operacionais na máquina B

Nenhuma delas muda o código nem o desenho experimental:

- **Docker Hub com limite de requisições (HTTP 429):** as imagens base vieram do espelho
  `mirror.gcr.io`, nas mesmas tags.
- **Toxiproxy:** o download do `ghcr.io` é bloqueado pela rede. Usamos a mesma versão, 2.12.0,
  compilada do código-fonte oficial (`github.com/Shopify/toxiproxy/v2`).
- **Serviço e orquestrador:** os containers de build não têm acesso à internet. Os binários
  foram compilados no host com Go 1.25, depois de `go vet` e dos 40 testes unitários passarem.
- **`nofile` = 20000** em vez de 65536: é o teto do sandbox. Bastou, já que o k6 não descartou
  nenhuma iteração.
- A VM foi recolhida uma vez durante a campanha (sessão ociosa), matando a execução 4. A
  varredura foi retomada. Execuções sem `manifesto.json` são refeitas, as completas são
  reaproveitadas, como previsto em `methodology.md` §10.

## Calibração

| | Máquina A | Máquina B | Diferença |
|---|---:|---:|---:|
| Capacidade μ | 190,45 req/s | **197,04 req/s** | +3,5 % |
| Tempo de serviço Ŝ | 42,03 ms | **40,59 ms** | −3,4 % |
| p95 do tempo de serviço | 44,8 ms | 44,8 ms | 0 |
| Desvio entre as 3 janelas (μ / Ŝ) | 0,11 % / 0,07 % | 0,11 % / 0,07 % | — |
| μ contado × C/Ŝ (lei de Little) | 0,14 % | 0,06 % | — |
| W = 150 − Ŝ − 10 | 98 ms | **99 ms** | +1 ms |

**Leitura:** a máquina B é ~3,5 % mais rápida no tempo de serviço médio. O p95 é idêntico
(44,8 ms) porque ele é dominado pela consulta de 40 ms no Postgres, não pela CPU. As duas
calibrações passam nos mesmos critérios com folga parecida. O orquestrador deriva Ŝ e W da
calibração de cada máquina, então as políticas se ajustam sozinhas, que é exatamente o que o
desenho previa (`methodology.md` §7).

## Desfechos: máquina A × máquina B

Repetição 1, mesma célula, mesma semente. As células da máquina B vão sendo preenchidas à
medida que a campanha avança.

| Execução | Máquina A | Máquina B | Igual? |
|---|---|---|---|
| limite fixo · backoff · 20 % | recuperou, 0 s | recuperou, 0 s | ✅ |
| fila fixa · backoff · 50 % | presa: 0,7 % no fim, amplificação 4,04 | presa: 0 % no fim, amplificação 4,01 | ✅ |
| limite fixo · espera fixa · 50 % | recuperou, 1 s; 94,7 % no fim (impacientes 89,6 %) | recuperou, 1 s; 100 % no fim | ⚠️ desfecho igual, intensidade diferente |
| *(demais 93 execuções)* | | *em andamento* | |

**Primeira divergência.** Na máquina A, o limite fixo a 50 % deixou 10 % dos impacientes sem
resposta no fim. Na máquina B, 0 %. Isso é coerente com o mecanismo descrito no
`results.md` §3.2: o limite fixo prejudica o impaciente quando W é maior que a espera útil dele.
Com Ŝ menor na máquina B, sobra mais folga, e as rajadas que empurram a fila além de 48 ms de
espera ficam mais raras.

## O que esperamos ver, e o que contaria como problema

- **Esperado:** os mesmos desfechos (recupera / preso / cai antes da falha) na maioria das
  células, porque o mecanismo depende de razões (espera da fila cheia ÷ timeout, (N+1)·λ ÷ μ) que
  quase não mudam com 3,5 % de diferença em μ.
- **Sensível:** células perto de uma fronteira, como o limite fixo a 50–70 % e a fila fixa a
  90 % antes da falha. Elas dependem de rajadas e de engasgadas do ambiente, e a máquina B tem o
  dobro de vCPUs.
- **Problema:** prazo por pedido ou LIFO ficarem presos, ou a fila fixa se recuperar a 50–70 %.
  Isso indicaria que o resultado principal depende da máquina.

## Conclusão (a completar ao fim da campanha)

*[preenchido ao fim das 96 execuções: quantas células concordam, quais divergem, e se as
conclusões do results.md se mantêm]*
