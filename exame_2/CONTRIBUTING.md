# Guia para mexer na bancada do experimento 2

Este guia é para quem nunca abriu este código e quer entender como ele está organizado, a ponto
de conseguir mudar alguma coisa sozinho. Ele tem três partes:

1. **[Intuição](#parte-1--intuição-a-bancada-é-um-restaurante)**: a bancada inteira explicada como
   um restaurante, sem código.
2. **[Aprofundamento](#parte-2--aprofundamento-o-código-peça-por-peça)**: cada peça, os arquivos
   e as funções que importam.
3. **[Receitas](#parte-3--receitas-para-mexer)**: o passo a passo para as mudanças mais comuns.

Se você só tem 5 minutos, leia a Parte 1 e o [mapa do slide](#o-mapa-do-slide-protótipo).

---

## Parte 1 · Intuição: a bancada é um restaurante

### O restaurante

Imagine um restaurante pequeno:

- **A cozinha** tem **8 cozinheiros** e um **trilho com espaço para 64 comandas**.
- Cada prato leva uns **40 ms** de preparo, porque o cozinheiro precisa buscar um ingrediente na
  **despensa**.
- **Os clientes chegam num ritmo fixo**, como se uma catraca soltasse um por vez, quer a cozinha
  esteja tranquila ou pegando fogo.
- Há dois tipos de cliente:
  - 🐇 **o impaciente**, que espera no máximo **100 ms** pela comida;
  - 🐢 **o paciente**, que espera até **200 ms**.
- Quem desiste **não vai embora de vez**: volta para a fila e **pede de novo** (o *retry*), até
  3 vezes.

A regra que torna tudo interessante:

> **Ninguém avisa a cozinha quando um cliente desiste.** A comanda fica no trilho, e algum
> cozinheiro vai preparar aquele prato para uma mesa vazia.

### O desastre (a falha metaestável)

Um dia, por **20 segundos**, o corredor até a despensa fica bloqueado e cada ida demora
**200 ms a mais**. Os pratos atrasam, os clientes desistem e pedem de novo. O trilho enche de
comandas repetidas e de **comandas de fantasmas**, de gente que já foi embora.

O corredor libera, mas **a cozinha não volta ao normal**. Os cozinheiros passam o dia preparando
pratos de fantasmas. Quando chegam ao pedido de um cliente de verdade, ele também já desistiu, e
também pediu de novo. A causa sumiu, mas o problema se sustenta sozinho. Isso é uma
**falha metaestável**.

### A pergunta do experimento

**Que regra o chefe da cozinha deve seguir para escolher a próxima comanda do trilho**, de modo
que a cozinha se recupere? Testamos 4 regras, as **políticas de fila**:

| # | Regra do chefe | Nome no código |
|---|---|---|
| 1 | "Pega sempre a mais antiga e faz, sem perguntas." | `fifo` |
| 2 | "Pega a mais antiga, mas se ela espera há mais de W = 98 ms, joga fora." | `timebound` |
| 3 | "Cada comanda diz até quando o cliente espera. Se não dá tempo de fazer, joga fora." | `deadline` |
| 4 | "Na correria, atende a comanda **mais nova** e joga fora as velhas demais." | `lifo` |

E duas formas de o cliente pedir de novo, as **políticas de retry**:

| Retry | Comportamento | Nome no código |
|---|---|---|
| Espera fixa | sempre espera 50 ms e pede de novo | `fixed` |
| Backoff com jitter | espera cada vez mais, com um sorteio | `backoff` |

### Quem é quem na bancada

| No restaurante | Na bancada | Pasta |
|---|---|---|
| A catraca que solta clientes | **k6**, o gerador de carga | `carga/` |
| A cozinha e o chefe | **o serviço em Go**, com workers, fila e política | `servico/` |
| A despensa | **Postgres**, o banco de dados | `infra/postgres/` |
| O corredor até a despensa (onde aparece o bloqueio) | **Toxiproxy**, um proxy que injeta atraso | `infra/toxiproxy/` |
| Uma câmera que fotografa o placar a cada 1 s | **Prometheus** | `infra/prometheus/` |
| O diretor com cronômetro, que monta cada ensaio | **o orquestrador em Go** | `orquestrador/` |
| O jornalista que escreve a matéria no fim | **a análise em Python** | `analise/` |
| O professor que faz as contas no quadro | **o modelo e o simulador** | `modelo/` |

### Um ensaio, do começo ao fim

O diretor (orquestrador) monta cada ensaio sempre igual:

```
 0 s ─── 10 s ────────────── 40 s ──────── 60 s ──────────────────────────── 150 s
 │ aquece │  regime normal   │  BLOQUEIO   │  recuperação: a cozinha volta?   │
 │        │  (custo da regra │  +200 ms no │                                  │
 │        │  em dia calmo)   │  corredor   │                                  │
```

1. Escolhe a regra do chefe (sem fechar o restaurante, só avisando a cozinha).
2. Abre a catraca com uma carga (20, 50, 70 ou 90 % do que a cozinha aguenta).
3. Aos 40 s bloqueia o corredor e aos 60 s libera.
4. Espera 90 s e vê se a cozinha voltou a servir 95 % do que servia antes.
5. Guarda as fotos da câmera num CSV.

Isso se repete para **4 regras × 2 retries × 4 cargas = 32 ensaios**, três vezes (96 no total).

### Por que a comparação é justa

- **Os mesmos clientes para todas as regras.** A ordem e o tipo de cada cliente saem de um sorteio
  com semente fixa, então as 8 combinações de uma mesma carga recebem exatamente a mesma fila de
  clientes novos.
- **A cozinha não é reconstruída entre ensaios**: só a regra muda.
- **Se o prédio tremeu, o ensaio não vale.** Se a máquina congelou mais de 50 ms durante o
  ensaio, ele é descartado e repetido.

---

## Parte 2 · Aprofundamento: o código peça por peça

### O mapa do slide "Protótipo"

O slide **"Protótipo · A bancada e como ela foi testada"** da [apresentação](apresentacao.html)
tem 7 caixas. Cada uma é uma pasta:

```
k6 (carga/) ──HTTP──► serviço Go (servico/) ──SQL──► Toxiproxy (infra/toxiproxy/) ──► Postgres (infra/postgres/)
                          │ /metrics
                          ▼
                   Prometheus (infra/prometheus/), coleta a cada 1 s  ◄── o k6 também envia métricas
                          ▲
 orquestrador Go (orquestrador/): troca a política, liga o atraso, roda o k6, lê o Prometheus, grava CSV
                          │
                          ▼
                análise Python (analise/): tabelas e figuras
```

| Caixa do slide | Onde está |
|---|---|
| Carga · k6 (carga aberta, 2 classes, timeout + retry) | `carga/script.js` |
| Serviço · Go (8 workers, fila 64, 4 políticas de fila) | `servico/main.go`, `servico/server.go` |
| Toxiproxy (+200 ms por 20 s) | `infra/toxiproxy/`, comandado por `orquestrador/clientes.go` |
| Postgres 17 (consulta de ~40 ms) | `infra/postgres/init.sql` |
| Prometheus (séries a cada 1 s) | `infra/prometheus/prometheus.yml` |
| Orquestrador · Go (política, k6, falha, CSV + manifesto) | `orquestrador/` |
| Análise · Python (tabelas e figuras) | `analise/analise.py`, `analise/estatistica.py` |
| "40 testes unitários + go vet, dentro do build" | `servico/server_test.go`, `orquestrador/metricas_test.go` e os `Dockerfile` |

Tudo sobe em containers pelo `docker-compose.yml`. **No seu computador só precisa de Docker.** Go,
k6, Python, Postgres e Prometheus rodam dentro das imagens.

### Peça 1 · O serviço (`servico/`): a cozinha

É a peça mais importante: é **o sistema sob teste**.

**`servico/main.go`** é a porta de entrada. Ele lê a configuração das variáveis de ambiente
(`WORKERS=8`, `QUEUE=64`, `SERVICE_MS=40`, a política inicial, W, Ŝ…), conecta ao Postgres
**pelo Toxiproxy** (`DB_URL` aponta para `toxiproxy:5433`, e não direto para o banco) e sobe o
servidor HTTP. O comentário no topo do arquivo explica as duas escolhas deliberadas sem as quais o
laço de retry não poderia acontecer:

1. a fila é grande o bastante para um pedido esperar mais que o timeout do cliente;
2. **não há cancelamento**: se o cliente desiste, o pedido é processado mesmo assim.

**`servico/server.go`** tem toda a lógica. Leia nesta ordem:

| Função | O que faz | No restaurante |
|---|---|---|
| `handleWork` | Recebe um pedido HTTP em `/work` e põe na fila. Fila cheia dá 503, exceto no LIFO, que expulsa o pedido mais antigo para o novo entrar. Lê dois headers: `X-Client-Class` (impaciente ou paciente) e `X-Budget-Ms` (o prazo do cliente). | o garçom pendura a comanda no trilho |
| `worker` | Um laço infinito, um por worker: espera ter pedido na fila, chama `retirar` e então processa ou descarta. | o cozinheiro |
| **`retirar`** | **O coração do experimento.** Um `switch` sobre a política decide *quem sai da fila* e *se é descartado*. É aqui que as 4 regras do chefe estão escritas. | o chefe escolhendo a comanda |
| `processar` | Faz o trabalho (a consulta ao banco) e classifica o resultado: `useful` (o cliente ainda esperava), `wasted` (o cliente já tinha desistido) ou `error`. | o prato sai, para uma mesa cheia ou vazia |
| `descartar` | Responde 503 a quem a política jogou fora e conta o motivo. | a comanda vai para o lixo |
| `handleAdmin` | `POST /admin/policy?name=lifo&max_wait=98...` troca a política **com o serviço no ar**. | o diretor avisa o chefe da nova regra |
| `Handler` | As rotas: `/work`, `/state`, `/admin/policy`, `/healthz` e `/metrics`. | |

Os motivos de descarte (`full`, `evicted`, `expired`, `deadline`, `stale`) e os nomes das
políticas (`fifo`, `timebound`, `deadline`, `lifo`) são constantes no topo do `server.go`.

**O que o serviço publica em `/metrics`** (o Prometheus lê daqui):

| Métrica | Significado |
|---|---|
| `svc_requests_received_total{class}` | pedidos que chegaram |
| `svc_requests_completed_total{outcome, class}` | processados, com `outcome` = `useful`, `wasted` ou `error` |
| `svc_requests_dropped_total{reason, class}` | descartados, por motivo |
| `svc_requests_needless_drops_total{class}` | descartes desnecessários: pedidos que ainda daria tempo de atender |
| `svc_lifo_active` | 1 quando o LIFO está em modo de sobrecarga |

### Peça 2 · A carga (`carga/script.js`): os clientes

É um script do **k6**. Cada *iteração* do k6 é um **pedido lógico**, que pode virar até 4
tentativas HTTP (1 + 3 retries).

- **Carga aberta**: o executor `constant-arrival-rate` inicia pedidos novos a uma taxa fixa
  (`RATE`), sem esperar respostas. É isso que deixa o laço fechar: se a carga esperasse as
  respostas, ela diminuiria sozinha quando o serviço ficasse lento.
- **Números comuns**: a função `uniforme(semente, iteração)` é um hash. A classe e o instante de
  chegada de cada pedido saem dela, não de `Math.random`. Mesma `SEED` = mesmos clientes.
- **O goodput é medido aqui, e não no servidor**, porque só o cliente sabe se a resposta chegou
  dentro do prazo. Os contadores são `bancada_originados`, `bancada_tentativas`,
  `bancada_sucesso`, `bancada_timeouts`, `bancada_rejeitados` etc.
- Todos os parâmetros vêm de variáveis de ambiente (listadas no topo do arquivo), definidas pelo
  orquestrador.

### Peça 3 · A infraestrutura (`infra/`): despensa, corredor e câmera

São arquivos curtos de configuração, cada um com seu `Dockerfile`:

- **`infra/postgres/init.sql`** cria a tabela `itens` com 10 mil linhas. Cada pedido lê uma linha
  com `pg_sleep`, então o tempo de serviço é conhecido.
- **`infra/toxiproxy/toxiproxy.json`** cria o proxy `pg`, que escuta em `:5433` e repassa para
  `postgres:5432`. Ele não faz nada sozinho: o orquestrador liga e desliga o atraso pela API dele.
- **`infra/prometheus/prometheus.yml`** coleta o serviço com `scrape_interval: 1s`. Essa é a
  resolução de todas as séries.

### Peça 4 · O orquestrador (`orquestrador/`): o diretor

É um programa Go que roda **dentro da imagem do k6**, porque chama o k6 como subprocesso (veja o
`orquestrador/Dockerfile`). Os arquivos, do mais geral ao mais específico:

| Arquivo | Papel |
|---|---|
| `main.go` | Os subcomandos `calibrar`, `cenario`, `varredura`, `tudo` e `reprocessar`, e as flags. |
| `varredura.go` | A **calibração** (`calibrar`, `medirMu`): satura o serviço 3 vezes para medir a capacidade μ e o tempo de serviço Ŝ. E a **varredura** (`PlanoVarredura.cenarios`, `varrer`): o produto cartesiano filas × retries × cargas × repetições, em ordem embaralhada. Pula execuções que já têm `manifesto.json`, por isso dá para interromper e retomar. Grava o `resumo.csv`. |
| `cenario.go` | **Uma execução**. `executarCenario` é a linha do tempo da Parte 1 em código. Também tem a `vigiaPausas` (o detector de congelamento) e `coletarSeries`, `gravarCSV` e `resumir`. |
| `clientes.go` | Os clientes HTTP: `Toxiproxy` (`LigarAtraso`, `DesligarAtraso`), `Prometheus` (consultas de séries) e o serviço (`definirPolitica`, `aguardarOcioso`). |
| `metricas.go` | **Só funções puras** sobre séries de 1 s: fases, deltas, médias, `tempoRecuperacao`, quantis, `classificar`. Tudo o que vira número no relatório passa por aqui, e é isso que os testes cobrem. |
| `json.go` | Um `MarshalIndent` que aceita NaN (vira `null`) para não perder um manifesto por causa de uma divisão por zero. |
| `afinidade_linux.go` | Prende cada thread da vigia de pausas numa CPU. |

**`executarCenario`, passo a passo:**

1. limpa o Toxiproxy e espera o serviço ficar ocioso (`aguardarOcioso`);
2. troca a política de fila (`definirPolitica` chama `POST /admin/policy`);
3. inicia o k6 (`execK6`) e marca o t0 no primeiro pedido que chega;
4. liga a vigia de pausas;
5. aos 40 s, `LigarAtraso(200)`; aos 60 s, `DesligarAtraso()`;
6. espera o fim (150 s) e confere se o Prometheus tem todas as amostras;
7. se a maior pausa passou de 50 ms, **descarta a execução**;
8. coleta as séries, grava `series.csv`, calcula o `Resumo` e grava `manifesto.json`.

**O que sai** fica em `resultados/` (veja a tabela no [README](README.md#output)):
`series.csv` (uma linha por segundo), `manifesto.json` (tudo o que é preciso para reproduzir a
execução) e `resumo.csv` (uma linha por execução).

### Peça 5 · A análise (`analise/`): o jornalista

- **`analise.py`** lê `resumo.csv` e as `series.csv` e gera as figuras por carga (os slides de
  20, 50, 70 e 90 %) e as tabelas. No topo ficam as listas `FILAS` e `NOME_FILA`, com a ordem e os
  nomes bonitos das políticas.
- **`estatistica.py`** agrega as 3 repetições (média ± desvio-padrão).

### Peça 6 · O modelo (`modelo/`): o professor no quadro

É uma validação independente da bancada: `simulador.py` implementa as mesmas 4 políticas num
simulador de eventos discretos, só com a biblioteca padrão do Python, em ~3 s e sem Docker. Se a
bancada e o simulador discordam, um dos dois tem bug. Veja o [modelo/README.md](modelo/README.md).

### Os dois scripts de entrada

- **`bancada.sh`** embrulha o Docker Compose. `./bancada.sh` sem argumentos mostra a ajuda.
- **`experimentos.sh`** roda a campanha inteira: calibração, 96 execuções e análise (~4 h 30).

### Como os testes funcionam

**Os testes rodam dentro do build das imagens.** O `Dockerfile` do serviço e o do orquestrador têm
`RUN go vet ./... && go test -count=1 ./...`. Se um teste falha, a imagem não é construída e
nenhum experimento roda com código quebrado.

- `servico/server_test.go` (17 testes): um teste por política em `TestRetirar*`, fila cheia,
  desperdício na fila fixa, troca de política no ar, capacidade = C/T, métricas expostas.
- `orquestrador/metricas_test.go` (23 testes): as funções puras de `metricas.go`, o plano da
  varredura, o resumo e o JSON com NaN.

```sh
./bancada.sh testes                           # pelo Docker, como no experimento
cd servico && go test ./...                   # direto, se você tem Go instalado
cd orquestrador && go test ./...
```

### Convenções

- **Código, comentários, logs e tabelas em português**; os documentos `PLAN.md`, `methodology.md`
  e `results.md` estão em inglês.
- **Lógica que vira número vai para função pura e ganha teste.** No orquestrador isso fica em
  `metricas.go`; no serviço, `retirar` é testável sem rede (veja `servidorParado` e `jobCom` nos
  testes).
- **Nada roda no host além do Docker.** Se você precisa de uma ferramenta nova, ela entra num
  `Dockerfile`.
- Os nomes curtos das políticas (`fifo`, `timebound`, `deadline`, `lifo`, `fixed`, `backoff`) são
  os mesmos em todas as peças: serviço, orquestrador, CSV e análise.

---

## Parte 3 · Receitas para mexer

### Rodar uma única execução para ver tudo funcionando

```sh
./bancada.sh cenario -fila lifo -politica fixed -carga 70
```

Leva ~3 minutos. O log mostra se recuperou. Os arquivos aparecem em `resultados/`.

### Mudar um parâmetro do serviço (workers, fila, tempo de serviço)

São variáveis de ambiente do serviço no `docker-compose.yml`, com padrão:

```sh
WORKERS=16 QUEUE=128 ./bancada.sh cenario -fila fifo -politica fixed -carga 50
```

Depois de mudar `WORKERS` ou `SERVICE_MS`, **recalibre** (`./bancada.sh calibrar`), porque μ e Ŝ
mudam e W é derivado de Ŝ.

### Mudar a falha, as fases ou os clientes

São flags do orquestrador, todas em `flagsComuns` (`orquestrador/main.go`). As mais usadas:

| Flag | Padrão | O que muda |
|---|---|---|
| `-atraso-ms` | 200 | latência injetada no banco |
| `-aquecimento`, `-regime`, `-atraso`, `-recuperacao` | 10, 30, 20, 90 | duração de cada fase, em s |
| `-classes` | `impatient:100:0.5,patient:200:0.5` | classes de cliente (`nome:timeout_ms:fração`) |
| `-fixed-delay` | 50 | espera do retry `fixed`, em ms |
| `-backoff-base`, `-backoff-teto` | 100, 1000 | o backoff exponencial, em ms |
| `-max-wait` | derivado de Ŝ (98 ms) | o W das políticas `timebound` e `lifo` |
| `-detector` | `empty` | o detector de sobrecarga do LIFO (`empty` ou `codel`) |

Exemplo: `./bancada.sh cenario -fila timebound -politica fixed -carga 70 -max-wait 48`.

### Adicionar uma política de fila nova

Este é o caso mais comum. Toque nestes lugares, nesta ordem:

1. **`servico/server.go`**
   - crie a constante (por exemplo, `PolNova = "nova"`) e inclua-a em `politicasValidas`;
   - adicione um `case PolNova:` em **`retirar`**, que devolve o pedido escolhido e o `motivo` do
     descarte (`""` para processar). Se precisar de um motivo novo, crie a constante e inclua-a em
     `motivos`.
   - Se a política tiver um parâmetro, adicione o campo em `Politica` e trate-o em `handleAdmin`.
2. **`servico/server_test.go`**: escreva um `TestRetirarNova` no estilo dos outros
   (`servidorParado` + `jobCom`) e um teste de ponta a ponta, se fizer sentido.
3. **`orquestrador/main.go`**: inclua o nome em `listaFilas` e no texto de ajuda da flag
   `-fila`/`-filas`.
4. **`orquestrador/varredura.go`**: inclua o nome em `ordemFila` (a ordem das linhas no
   `resumo.csv`).
5. **`orquestrador/cenario.go`**: se criou um motivo de descarte, inclua-o na lista de `extras()`
   para virar coluna no CSV.
6. **`analise/analise.py`**: inclua o nome em `FILAS` e em `NOME_FILA`.
7. *(opcional)* **`modelo/simulador.py`**: implemente-a também, para ter a previsão teórica.

Depois: `./bancada.sh testes` e um `./bancada.sh cenario -fila nova -politica fixed -carga 70`.

### Adicionar uma métrica nova

1. **Serviço**: registre o contador ou medidor em `NewServer` (`servico/server.go`) e incremente
   onde acontece.
2. **Orquestrador**: inclua um `extra{Coluna: ..., Seletor: ...}` em `extras()`
   (`orquestrador/cenario.go`). A coluna aparece no `series.csv`.
3. Se ela precisa virar um número por execução, some ou faça a média dela em `resumir` ou
   `resumirExtras` e adicione o campo em `Resumo`. Se a conta não é trivial, ponha-a em
   `metricas.go` com teste.
4. **Análise**: use a coluna nova em `analise.py`.

### Refazer o CSV sem rodar de novo

Se você mudou só o cálculo (por exemplo, em `metricas.go` ou `resumir`) e o Prometheus ainda tem
os dados:

```sh
./bancada.sh reprocessar -nome principal
```

### Desligar tudo

```sh
./bancada.sh derrubar
```

---

## Por onde começar a ler o código

Se você vai explicar o código na arguição, ou quer mexer pela primeira vez, leia nesta ordem:

1. `servico/server.go`: `retirar`, depois `handleWork` e depois `processar`. É **o experimento**.
2. `orquestrador/cenario.go`: `executarCenario`. É **a linha do tempo de uma execução**.
3. `carga/script.js`: a função `default`. É **o cliente com retry**.
4. `orquestrador/metricas.go`: `tempoRecuperacao` e `classificar`. É **o que conta como
   "recuperou"**.

O resto é infraestrutura para essas quatro coisas acontecerem de forma repetível.
