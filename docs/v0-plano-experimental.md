# V0 — Esqueleto experimental

**Engineering Resilient Distributed Systems**
Proposta de prova de conceito · documento de trabalho · v0.1

---

## Objetivo

Percorrer **uma vez, de ponta a ponta**, a cadeia experimental completa:

```
modelo de faltas → injeção → modelo de carga → cenário → observação → mitigação → nova medição
```

O objetivo do V0 **não é produzir resultado científico**. É validar que o ferramental funciona junto, que a medição é confiável e reprodutível, e que existe um caminho executável do experimento à conclusão. Só depois disso vale investir em cenários interessantes.

Analogia: é um *walking skeleton* — o menor sistema possível que atravessa todas as camadas.

---

## 1. Sistema de exemplo

Deliberadamente mínimo. Dois serviços escritos por nós, quatro componentes de prateleira.

```
┌────────┐    ┌──────────┐    ┌───────────┐    ┌─────────┐
│   k6   │───▶│ gateway  │───▶│ toxiproxy │───▶│ backend │
└────────┘    └──────────┘    └───────────┘    └─────────┘
   carga       política de       injeção de       gargalo
   aberta         retry           faltas        determinístico
                     │                               │
                     └──────────┬────────────────────┘
                                ▼
                        ┌──────────────┐    ┌─────────┐
                        │  prometheus  │───▶│ grafana │
                        └──────────────┘    └─────────┘
```

### `backend` — o gargalo

Node.js + TypeScript. Um único endpoint. A capacidade é **deliberadamente determinística**:

- Semáforo de concorrência fixa `C` (ex.: 8 requisições simultâneas)
- Cada requisição consome `T` ms de trabalho simulado (ex.: 40 ms)
- Fila de espera **limitada** — ao encher, responde 503

Capacidade teórica: `μ = C / T` = 8 / 0,04 s = **200 req/s**.

> **Decisão de projeto:** trabalho simulado por espera controlada, não por carga real de CPU. O motivo é que μ precisa ser conhecido e estável para que a camada analítica do TG tenha um valor de referência. Carga real de CPU varia com o escalonador do host e destrói a reprodutibilidade.

### `gateway` — onde vivem as mitigações

Node.js + TypeScript. Recebe do k6, chama o backend, aplica a política de retry. **Toda mitigação é configurável por variável de ambiente**, para que trocar de arm de experimento não exija recompilar:

| Variável | Valores | Descrição |
|---|---|---|
| `RETRY_POLICY` | `none` \| `immediate` \| `backoff` \| `budget` | Disciplina de retry |
| `RETRY_MAX` | inteiro | Tentativas máximas |
| `BACKOFF_BASE_MS` | inteiro | Base do backoff exponencial |
| `RETRY_BUDGET_RATIO` | float | Fração do tráfego reservada a retry |
| `CLIENT_TIMEOUT_MS` | inteiro | Timeout por tentativa |
| `REQUEST_DEADLINE_MS` | inteiro | Prazo total — define goodput |

### Componentes de prateleira

- **Toxiproxy** entre gateway e backend — injeção de latência e corte de conexão
- **k6** — geração de carga
- **Prometheus** — coleta, intervalo de raspagem de **1 s**
- **Grafana** — visualização

Tudo em um único `docker-compose.yml`.

---

## 2. Modelo de faltas do V0

Escopo fechado em **duas** classes. As demais ficam explicitamente para depois.

| Classe | Injeção | Ferramenta |
|---|---|---|
| **Timing** (atraso) | +200 ms em toda resposta do backend | Toxiproxy, toxic `latency` |
| **Crash-recovery** | matar e reiniciar o container do backend | `docker kill` + `docker start` |

**Fora do V0:** perda de mensagem, corrupção, pressão de recurso via cgroups, faltas bizantinas, faltas em chamada RPC individual.

---

## 3. Modelo de carga do V0

k6 com executor **`constant-arrival-rate`**.

> **Decisão de metodologia, e a mais importante do V0:** executor de taxa de chegada constante, nunca `constant-vus`. Um gerador fechado para de enviar quando o servidor engasga e produz *coordinated omission* — as estatísticas saem otimistas e o colapso fica invisível. Com `constant-vus` este experimento é incapaz de observar o fenômeno de interesse.

Perfil de carga:

| Fase | Duração | Taxa | Observação |
|---|---|---|---|
| Aquecimento | 30 s | 60 % de μ | Estabilizar |
| Regime | 60 s | 80 % de μ | Estado *vulnerável* |
| Perturbação | 30 s | 80 % de μ | Falta injetada aqui |
| Recuperação | 120 s | 80 % de μ | **A fase que interessa** |

A fase de recuperação é longa de propósito: é nela que se distingue degradação transitória de estado metaestável.

---

## 4. Instrumentação

Métricas expostas em `/metrics` desde a primeira linha de código. Instrumentar depois nunca acontece.

**No gateway:**

| Métrica | Tipo | Rótulos |
|---|---|---|
| `requests_total` | counter | `result` = ok \| timeout \| shed \| error |
| `retries_total` | counter | `attempt` |
| `responses_total` | counter | `freshness` = fresh \| stale |
| `request_duration_seconds` | histogram | — |
| `inflight_requests` | gauge | — |
| `retry_budget_tokens` | gauge | — |

**No backend:**

| Métrica | Tipo |
|---|---|
| `queue_depth` | gauge |
| `active_workers` | gauge |
| `service_time_seconds` | histogram |
| `shed_total` | counter |

**Derivadas no Grafana:**

- **Goodput** = `responses_total{freshness="fresh"}` por segundo
- **Throughput** = total de respostas por segundo
- **Trabalho desperdiçado** = throughput − goodput
- **Taxa de amplificação** = `retries_total` / `requests_total`

Painel com quatro gráficos: goodput vs. throughput · amplificação por retry · profundidade da fila · latência em p50/p95/p99.

---

## 5. Cenários do V0

### V0-1 — Atraso transitório (principal)

**Falta:** latência de +200 ms via Toxiproxy durante a fase de perturbação.
**Carga:** 80 % de μ.
**Arms:** `RETRY_POLICY=immediate` e `RETRY_POLICY=backoff`.

**Hipótese de estado estável:** o goodput retorna a ≥ 95 % do valor de regime dentro de 30 s após a remoção da falta.

**Predição:** a hipótese falha com `immediate` e se sustenta com `backoff`.

### V0-2 — Crash e reinício (secundário)

**Falta:** `docker kill backend`, 5 s parado, `docker start`.
**Arms:** os mesmos dois.

**Hipótese:** o goodput retorna ao regime dentro de 60 s após o backend voltar.

Se o tempo apertar, V0-2 fica para depois. V0-1 é suficiente para validar a cadeia.

---

## 6. Critérios de conclusão

O V0 está pronto quando **todos** forem verdadeiros:

1. **μ medido e reprodutível.** Uma rotina de calibração determina μ empiricamente, e três execuções concordam dentro de ± 5 %.
2. **Execução por um comando.** `make run SCENARIO=v0-1 POLICY=immediate` sobe o ambiente, roda o perfil, injeta a falta no instante correto, exporta os dados e derruba tudo.
3. **Saída em arquivo.** Séries temporais em CSV/JSON no diretório `results/`, com um manifesto de execução registrando configuração, versões e horário. Sem isso não há reprodutibilidade.
4. **Sanidade pela lei de Little.** `inflight ≈ λ × latência média` dentro de uma margem razoável. Se não fecha, a medição está errada e nenhum resultado vale.
5. **Os dois arms produzem curvas distinguíveis** no cenário V0-1.

**Explicitamente NÃO é critério:** demonstrar uma falha metaestável. Se aparecer, é bônus e antecipa o cronograma. Se não aparecer, o V0 continua bem-sucedido — significa que o espaço de parâmetros precisa ser explorado, o que é resultado válido e já é conteúdo do TG.

---

## 7. Fora do escopo do V0

Lista tão importante quanto a de escopo, porque é onde o tempo vaza:

- Kubernetes, k3s, service mesh — Compose basta e o fenômeno não depende de orquestrador
- Nuvem paga — reproduz em laptop; nuvem troca tempo de pesquisa por configuração de IAM
- Mais de dois serviços próprios — cadeia mais longa depois, quando houver pergunta que exija
- Circuit breaker, load shedding no cliente, propagação de deadline — mitigações da fase seguinte
- Replicação — fase seguinte
- Modelo analítico de filas — depois de existir dado empírico com que comparar
- Frontend, autenticação, banco de dados, qualquer coisa de produto

> Regra de bolso: toda vez que der vontade de melhorar a aplicação, é sinal de fuga do experimento. A aplicação é instrumento de laboratório, não produto.

---

## 8. Riscos de execução

| Risco | Mitigação |
|---|---|
| *Coordinated omission* invalida tudo | `constant-arrival-rate`, verificado no início |
| k6 disputa CPU com os serviços e distorce μ | Limitar CPU por container no Compose; conferir que a calibração de μ é estável |
| Raspagem de 1 s ainda perde transiente | Histogramas na aplicação; exportar também o resumo próprio do k6 |
| Overhead do Toxiproxy contamina a medição | Medir baseline **com** Toxiproxy no caminho e sem toxic ativo |
| Relógio do container e do host divergem | Registrar timestamps de um único relógio; alinhar séries pelo instante da injeção |
| Fila ilimitada esconde o *shedding* | Fila limitada desde o início, com contador de recusas |

---

## 9. Estrutura do repositório

```
tg-resilient-distributed-systems/
├── README.md
├── docs/
│   ├── fundamentacao-conceitual.md
│   ├── v0-plano-experimental.md
│   └── decisoes/               # registro de decisões (ADR)
├── services/
│   ├── gateway/
│   └── backend/
├── infra/
│   ├── docker-compose.yml
│   ├── prometheus.yml
│   └── grafana/dashboards/
├── experiments/
│   ├── calibrate.js            # mede μ
│   ├── v0-1-latency.js
│   ├── v0-2-crash.js
│   └── scenarios/              # orquestração da injeção
├── results/                    # saída versionada por execução
├── analysis/                   # notebooks de análise
└── Makefile
```

O diretório `decisoes/` é onde ficam as escolhas metodológicas com justificativa — por que `constant-arrival-rate`, por que trabalho simulado em vez de CPU real, por que fila limitada. Na hora de escrever o capítulo de metodologia, esse diretório já é o rascunho.

---

## 10. Sequência de execução

| Etapa | Entrega |
|---|---|
| 1 | Compose sobe; gateway e backend respondem; `/metrics` expõe dados |
| 2 | Prometheus raspa; painel do Grafana desenha as quatro visões |
| 3 | Calibração de μ; três execuções concordando |
| 4 | Toxiproxy no caminho; baseline sem toxic confirma overhead desprezível |
| 5 | V0-1 rodando por um comando, nos dois arms, com saída em arquivo |
| 6 | Checagem pela lei de Little; análise e gráfico comparativo |
| 7 | V0-2, se houver tempo |

Estimativa: **três fins de semana**, considerando dedicação parcial.

---

## Perguntas para a orientadora

1. A taxonomia de faltas do V0 (timing e crash-recovery) é o recorte inicial adequado, ou ela prefere começar por outra classe?
2. Ela concorda com trabalho simulado em vez de carga real de CPU, dado o ganho de reprodutibilidade e a perda de realismo?
3. Sobre a seleção de cenários — vale investigar STPA como instrumento sistemático para isso, dado que conecta com a linha dela?
4. Qual formato de registro ela prefere para as decisões metodológicas, considerando que isso vira o capítulo de metodologia?
5. Há preferência por HAProxy ou Envoy quando a camada de balanceamento entrar, ou fica a critério?