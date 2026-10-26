# 0001 — Escopo do semestre: Lab Exame CSC-27 / CE-288

**Status:** aceita · **Data:** 26/10/2026
**Equipe:** João Guerreiro · Lucas de Sousa · Marcelo Hippolyto · Pablo Carvalho
**Orientação:** Prof.ª Juliana de Melo Bezerra

## Contexto

A professora autorizou usar o tema do TG (falhas metaestáveis) como Lab Exame de
CSC-27 / CE-288. Com isso, o plano anual do TG (nota de escopo em `index.html`)
ganha um objetivo de curto prazo com regras próprias:

| Data | Entrega |
|---|---|
| 27/out | Apresentação da proposta (itens 1–4 do anexo: motivação, trabalhos relacionados, objetivo, proposta) |
| 24/nov ou 01/dez | Apresentação final (itens 3–6: objetivo, proposta, protótipo, conclusões) |
| 01/dez | Relatório (todos os itens do anexo) e códigos |

Regras do enunciado que pesam no escopo:

- A aplicação deve ser **completa e testada**, e **concebida pela equipe** — aplicações prontas não são permitidas.
- Todos os membros implementam e fazem análise técnica.
- IA só como apoio de compreensão e dúvidas de implementação; o uso deve ser declarado (item 8 do anexo).
- Docker recomendado; Go incentivado.

Restrições da equipe: menos de 4 h por pessoa por semana até o congelamento, cerca de
40 h no total. A semana de 17/nov tem a Prova 2.

## Decisão

O semestre entrega **uma volta da espiral**, sobre uma bancada construída do zero. O
resultado é uma medição, e não a demonstração obrigatória do fenômeno.

**Pergunta do exame.** Para quais taxas de chegada e políticas de retry o goodput não
volta ao regime depois que um atraso transitório no banco é removido?

**Critério de sucesso.** A bancada roda um cenário por um comando, μ é reprodutível e a
varredura delimita a região em que o sistema não se recupera. Se nenhuma região prender
o sistema, a entrega é a fronteira medida mais a condição que faltou, fundamentada.

### Bancada

```
k6 (carga aberta + retry) ──▶ serviço Go ──▶ Toxiproxy ──▶ Postgres
         │                        │
         └──────────▶ Prometheus ◀┘            orquestrador Go comanda k6 e Toxiproxy
```

- **Cliente — k6.** Executor `constant-arrival-rate`. O timeout e o retry ficam no
  cliente, porque fazem parte do modelo de carga. `maxVUs` fica folgado e
  `dropped_iterations` é monitorado: iteração descartada é omissão coordenada.
- **Serviço — Go.** Pool fixo de C workers. Cada pedido faz uma consulta simples ao
  Postgres via Toxiproxy mais T ms de trabalho simulado. A fila é limitada em Q e
  responde 503 quando cheia. O serviço não cancela pedidos abandonados pelo cliente.
  O pool de conexões ao banco é ≥ C, para que o gargalo seja o pool do serviço.
- **Banco — Postgres real** em container.
- **Injeção — Toxiproxy** entre serviço e banco, com toxic `latency` numa janela
  programada. O atraso segura o worker e derruba o μ efetivo.
- **Orquestrador — CLI em Go.** Sobe o cenário, inicia o k6, liga e desliga o atraso
  pela API do Toxiproxy no instante certo, coleta as séries do Prometheus e grava CSV
  com um manifesto da execução (configuração, versões, horário).
- **Prometheus** com raspagem de 1 s. Grafana é opcional.

### Experimento

- **Políticas de retry no cliente:** nenhuma · imediata · backoff exponencial com jitter.
- **Carga:** λ em 60, 70, 80 e 90 % de μ; timeout e atraso fixos.
- **Repetições:** 3 por combinação, o que dá 36 execuções de ~4 min.
- **Perfil de cada execução:** aquecimento → regime → atraso → recuperação longa.

### Métricas

| Métrica | Definição | Onde |
|---|---|---|
| Goodput | respostas de sucesso dentro do timeout, por segundo | k6 |
| Fator de amplificação | pedidos recebidos pelo serviço ÷ pedidos originados pelo cliente | serviço / k6 |
| Tempo de recuperação | tempo, após remover o atraso, até o goodput voltar a ≥ 95 % do regime | k6 |
| Profundidade da fila, latência p50/p95/p99 | — | serviço / k6 |

Há uma única definição de amplificação, que vale 1 em repouso. A definição antiga
(`retries_total / requests_total`) do `v0-plano-experimental.md` fica substituída.

### Condições de projeto para o laço poder fechar

1. **Q/μ + T > timeout.** Se a fila não consegue atrasar um pedido além do timeout, o
   enfileiramento nunca dispara retry e a metaestabilidade é impossível por construção.
2. **Sem cancelamento.** O serviço processa o pedido mesmo depois que o cliente desistiu.
   Sem trabalho desperdiçado não há amplificador.

Parâmetros iniciais, **a confirmar na calibração**: C = 8, T = 40 ms (μ ≈ 200 req/s),
atraso +200 ms, timeout 150 ms, Q = 64.

### Validação

- μ calibrado em 3 execuções, concordando dentro de ±5 %.
- Lei de Little: pedidos em voo ≈ λ × latência média.
- Baseline com o Toxiproxy no caminho e sem toxic ativo, para confirmar overhead desprezível.

### Divisão

| Pessoa | Parte |
|---|---|
| A | Serviço Go: pool, fila, acesso ao Postgres, `/metrics` |
| B | Carga no k6: taxa aberta, três políticas, métricas no cliente |
| C | Orquestrador Go: Toxiproxy, timing da injeção, export CSV e manifesto |
| D | Compose, calibração de μ, lei de Little, análise |

A distribuição dos nomes fica a cargo da equipe.

### Calendário

| Semana | Entrega |
|---|---|
| 27/out – 02/nov | Proposta apresentada; Compose sobe serviço, Toxiproxy, Postgres e Prometheus; k6 em taxa constante |
| 03 – 09/nov | μ calibrado; três políticas; injeção programada; export CSV; lei de Little |
| 10 – 14/nov | Varredura automatizada; **bancada congelada em 14/nov** |
| 17 – 23/nov | Prova 2; análise, gráficos, PPT final |
| até 01/dez | Relatório e códigos |

**Corte de emergência.** Se em 02/nov o Compose não estiver de pé com μ calibrado, o
experimento cai para duas políticas (imediata e backoff) e duas taxas.

## Fora do semestre (vai para o TG)

Crash e restart · perda de pacotes · Redis e cache frio · retry budget · circuit breaker ·
load shedding · interação entre mecanismos · modelo analítico de filas · replicação ·
gateway como serviço próprio · reprodutores do OSDI '22 (vetados como sistema de exemplo
pela regra de aplicações prontas).

## Consequências

- O `v0-plano-experimental.md` fica superado nos pontos em que diverge daqui: topologia
  (sem gateway, com Postgres), definição de amplificação e linguagem (Go em vez de
  Node/TypeScript).
- A nota de escopo (`index.html`) continua valendo como plano anual do TG. O semestre
  é a primeira volta dela.
- A taxonomia de falhas cita Avizienis para o vocabulário falta → erro → falha e
  Cachin, Guerraoui e Rodrigues para o modelo de falhas de processos e canais.
