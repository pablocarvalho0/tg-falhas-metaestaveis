# Fundamentação conceitual

**Engineering Resilient Distributed Systems**
Rascunho de capítulo · documento de trabalho · v0.1

> Escopo deste documento: consolidar os conceitos que estruturam o trabalho, no encadeamento
> modelo de faltas → injeção → modelo de carga → cenário → observação → mitigação.
> Serve como base para o capítulo de fundamentação teórica do TG.

---

## 1. Falta, erro e falha

A literatura de dependabilidade separa três coisas que a linguagem cotidiana confunde:

- **Falta** (*fault*) é a causa raiz: um bug, um cabo solto, um disco que morreu.
- **Erro** (*error*) é o estado interno incorreto que a falta produz.
- **Falha** (*failure*) é quando esse estado incorreto vaza para fora e o serviço deixa de cumprir o que prometeu.

A cadeia é falta → erro → falha, e nem toda falta chega ao fim. Se o sistema é resiliente, ela morre no caminho. Essa distinção é da taxonomia de Avizienis, Laprie, Randell e Landwehr, referência canônica da área.

Consequência direta para o trabalho: **medir resiliência é medir quantas faltas morrem antes de virar falha, e a que custo.**

---

## 2. Modelo de faltas

Um modelo de faltas é a declaração explícita de quais faltas se assume que existem. Isso importa porque "sistema resiliente" é frase incompleta: resiliente *a quê*? O modelo de faltas é a fronteira das garantias — fora dela, nada é prometido. Isso não é omissão, é honestidade de engenharia.

### 2.1 Taxonomia por severidade

| Classe | Comportamento | Exemplo |
|---|---|---|
| **Crash** | O componente para. Nada incorreto é produzido. | Processo morre, container é morto |
| **Omissão** | Algo que deveria acontecer não acontece. | Mensagem perdida na rede |
| **Timing** | Aconteceu, mas fora da janela esperada. | Mensagem atrasada, resposta lenta |
| **Bizantina** | Comportamento arbitrário, inclusive mentira coerente. | Nó corrompido ou malicioso |

A hierarquia é cumulativa: um modelo que tolera faltas bizantinas tolera as anteriores. Custo e dificuldade crescem na mesma ordem.

### 2.2 Dois sabores de crash

Distinção que muda muito a dificuldade do problema:

- **Crash-stop**: o processo morre e não volta.
- **Crash-recovery**: o processo morre e reinicia. Bem mais difícil, porque o processo que volta pode ter perdido estado, ou pior, pode voltar com estado obsoleto e agir sobre informação velha.

### 2.3 Modelos de sincronia

- **Síncrono**: existem limites conhecidos para atraso de mensagem e velocidade de processamento.
- **Assíncrono**: não existe limite algum.
- **Parcialmente síncrono**: existem limites, mas desconhecidos, ou válidos apenas após certo instante.

Quase todo sistema real é modelado como parcialmente síncrono. Assíncrono puro é pessimista demais para provar qualquer coisa útil; síncrono puro é otimista demais para ser verdade.

### 2.4 O resultado que amarra tudo

> **Num sistema assíncrono, é impossível distinguir um processo lento de um processo morto.**

Se a resposta não chegou, não há informação no sistema que diga se o outro lado caiu, se a rede engoliu a mensagem, ou se ele apenas está demorando. Esse é o núcleo do resultado de impossibilidade de Fischer, Lynch e Paterson (FLP).

A consequência de engenharia é que **timeout é obrigatório e é um chute**. E é do timeout que nasce o retry: o cliente desistiu de esperar por heurística e tentou de novo.

Portanto, o fenômeno estudado no capítulo 6 deste documento (falhas metaestáveis) não é um acidente de implementação — é uma consequência distante de um limite teórico de sistemas distribuídos.

---

## 3. Injeção de faltas

Injeção de faltas é provocar a falta deliberadamente para observar o efeito. A técnica vem de teste de hardware (injeção em nível de pino) e migrou para software como SWIFI, *software-implemented fault injection*.

### 3.1 Camadas de injeção

A camada escolhida determina que classe de falta é alcançável:

| Camada | Ferramenta | Faltas alcançáveis |
|---|---|---|
| Processo | `docker kill` / `docker start` | Crash-stop, crash-recovery |
| Rede | Toxiproxy, `tc`/netem | Omissão, timing, corrupção, limite de banda |
| Recurso | cgroups, `stress-ng` | Degradação (lentidão sem queda) |
| Aplicação | interceptação de RPC (ex.: Filibuster) | Falta em chamada individual |

A camada de recurso merece atenção: ela produz o caso mais perigoso, que é o componente que **não cai, só fica lento**. É o cenário que mais frequentemente gera colapso em cascata, e o mais difícil de detectar.

### 3.2 Explosão do espaço de faltas

Com *n* serviços, *k* classes de falta por chamada, combinações de até *m* faltas simultâneas e *l* níveis de carga, o número de experimentos cresce combinatorialmente. Não existe testar tudo.

Isso transforma a **seleção de cenários** na pergunta central da avaliação de resiliência, e não num detalhe de execução.

---

## 4. Modelo de carga

### 4.1 Fechado vs. aberto

- **Fechado**: N clientes virtuais em laço — envia, espera resposta, pensa, envia de novo. O número de clientes é fixo.
- **Aberto**: pedidos chegam a uma taxa fixa, independente do estado do servidor.

A diferença parece técnica e é conceitual. No modelo fechado, quando o servidor engasga, o gerador **automaticamente para de enviar**, porque cada cliente está bloqueado. O gerador se autoregula junto com o servidor.

> **Um gerador de carga fechado é incapaz de reproduzir sobrecarga aberta.** Ele esconde precisamente o fenômeno que este trabalho pretende estudar.

O artefato de medição resultante chama-se *coordinated omission* (termo popularizado por Gil Tene): os pedidos que teriam sido mais lentos nunca foram enviados, então as estatísticas saem otimistas e o sistema parece saudável enquanto está morrendo.

**Implicação prática:** no k6, usar o executor `constant-arrival-rate`, não `constant-vus`. É uma linha de configuração e muda o resultado qualitativamente. Isso será documentado como decisão de metodologia.

### 4.2 Parâmetros do modelo

- **Taxa de chegada** (λ)
- **Distribuição da chegada** — Poisson é o padrão de referência, mas tráfego real é em rajada, e rajada é o que provoca colapso
- **Tempo de pensar** — relevante apenas no modelo fechado

### 4.3 Lei de Little

$$L = \lambda W$$

O número médio de pedidos no sistema é igual à taxa de chegada multiplicada pelo tempo médio de permanência. É uma identidade, vale para qualquer disciplina de fila, e é a checagem de sanidade mais barata que existe para validar uma medição.

### 4.4 Capacidade e saturação

Com capacidade de serviço μ e chegada λ, enquanto λ < μ a fila é estável. Quando λ ≥ μ a fila cresce sem limite e a latência diverge. Essa é a fronteira que separa o regime normal do regime de sobrecarga, e todo o trabalho se passa em volta dela.

---

## 5. Cenário e chaos engineering

Chaos engineering se distingue de "quebrar coisas" por ser **experimento com hipótese**. O formato canônico (Basiri et al.):

1. Declarar uma **hipótese de estado estável** — uma métrica que deveria permanecer numa faixa.
2. Injetar um evento realista.
3. Verificar se a hipótese se sustentou.
4. Automatizar para rodar continuamente.
5. Minimizar o raio de explosão.

Um **cenário**, neste trabalho, é a combinação de um modelo de faltas instanciado com um modelo de carga instanciado, mais a hipótese de estado estável associada.

### 5.1 Como selecionar cenários

Pergunta aberta na literatura. Três abordagens conhecidas:

- **Enumeração guiada por estrutura** — percorrer o grafo de chamadas entre serviços e derivar sistematicamente as combinações de falta (abordagem do Filibuster).
- **Lineage-driven fault injection** — partir de uma execução bem-sucedida, reconstruir a linhagem de por que ela funcionou, e deduzir quais faltas poderiam tê-la impedido. Busca por contraexemplo em vez de sorteio (Alvaro et al., Molly).
- **Importação de método de safety** — FMEA e HAZOP fazem enumeração sistemática de modo de falha. STPA, por operar sobre a estrutura de controle, é candidato natural para identificar interações inseguras entre mecanismos de mitigação.

---

## 6. Observação

### 6.1 Vocabulários consolidados

Três esquemas para a mesma ideia; a recomendação é escolher um e ser consistente:

- **Four golden signals** (Google SRE): latência, tráfego, erros, saturação.
- **USE** (Gregg): utilização, saturação, erros — por recurso.
- **RED**: taxa, erros, duração — por serviço.

### 6.2 Duas armadilhas

- **Média de latência não informa.** O problema mora na cauda. São necessários percentis (p50, p95, p99, p99.9), e percentil não se obtém agregando percentis: exige histograma.
- **Janela de agregação grossa esconde transiente.** Amostragem de um minuto transforma um colapso de vinte segundos num ponto.

### 6.3 Métricas específicas para colapso por realimentação

As métricas óbvias não bastam. Latência e taxa de erro pioram durante qualquer sobrecarga, o que não distingue nada. O que evidencia o laço de realimentação:

| Métrica | Por que importa |
|---|---|
| **Taxa de retry / tráfego total** | Mede diretamente a amplificação |
| **Profundidade da fila** | Indicador antecedente: sobe antes da latência |
| **Goodput separado de throughput** | Revela servidor ocupado produzindo resposta que ninguém espera |

**Throughput** é quanto trabalho o servidor faz. **Goodput** é quanto desse trabalho serve para alguém. Um servidor pode estar 100% ocupado com goodput próximo de zero, porque todos os clientes já deram timeout e pediram de novo.

---

## 7. Mecanismos de mitigação

### 7.1 Retry e suas disciplinas

Retry existe para tolerar falta transitória, e é a mitigação mais comum. Também é o amplificador mais comum, porque cada tentativa é carga nova: serviço lento → mais retries → λ maior → serviço mais lento.

Disciplinas que quebram ou limitam o laço:

- **Backoff exponencial com jitter** — espera crescente entre tentativas, com aleatoriedade para evitar sincronização entre clientes.
- **Retry budget** — retries limitados a uma fração do tráfego normal, tipicamente via *token bucket*. Garante que a amplificação nunca exceda um fator conhecido.
- **Circuit breaker** — máquina de estados no cliente que, após N falhas, interrompe as tentativas por um período e depois sonda com tráfego reduzido.
- **Load shedding** — o servidor recusa trabalho na entrada para proteger o que já aceitou.
- **Propagação de deadline** — o pedido carrega quanto tempo ainda resta, evitando processamento de trabalho já expirado.

### 7.2 Replicação

Aumenta disponibilidade: a queda de uma cópia não derruba o serviço. O custo é consistência — as cópias precisam concordar, concordância custa comunicação, e sob partição de rede é preciso escolher entre responder com dado possivelmente obsoleto ou não responder. Esse é o conteúdo do teorema CAP.

Ponto relevante para este trabalho: **replicação também pode amplificar carga.** Escrita com quórum multiplica tráfego interno; *read repair* gera trabalho extra justamente quando o sistema está inconsistente, que é quando ele está sob estresse; rebalanceamento após uma queda gera pico de transferência no pior momento possível. Replicação é, portanto, candidata a amplificador tanto quanto retry.

---

## 8. A espiral

Cada mecanismo de mitigação é código novo, com estado novo, configuração nova e caminho de realimentação novo. Ele reduz a probabilidade da falha prevista e **aumenta a superfície de falha não prevista**. Testar fica mais difícil porque o espaço de estados alcançáveis cresce.

Duas referências sustentam isso:

- **Perrow**, *Normal Accidents*: em sistemas com complexidade interativa e acoplamento forte, acidentes deixam de ser anomalias e passam a ser propriedade do sistema.
- **Leveson**, STAMP/STPA: acidentes em sistemas complexos decorrem menos de componente quebrado e mais de interação insegura entre componentes que funcionam — inclusive interações envolvendo os próprios mecanismos de proteção.

Isso conecta o trabalho diretamente ao arcabouço de safety, e sugere que STPA possa ser instrumento para a seleção de cenários discutida em 5.1.

---

## 9. Falhas metaestáveis

Caso limite que integra todos os conceitos anteriores, e alvo de demonstração do trabalho.

### 9.1 Os três estados

- **Estável** — folga de capacidade confortável.
- **Vulnerável** — pouca folga. É o estado *desejável* em operação, porque folga é hardware ocioso. Praticamente todo sistema bem operado passa a maior parte do tempo aqui, deliberadamente.
- **Metaestável** — o laço de realimentação está fechado e se auto-sustenta.

### 9.2 A propriedade definidora

Quando o laço fecha, o sistema **para de depender da causa original**. O gatilho pode ter desaparecido e λ externo ter voltado ao normal; o sistema permanece degradado, porque a carga que o mantém ali é a que ele mesmo gera.

O estado é localmente estável e só é abandonado com intervenção externa — daí o nome, emprestado da física. É por isso que reiniciar o sistema funciona: drena as filas e quebra o laço à força.

### 9.3 Amplificadores além do retry

- **Cache frio** — cache esvazia, tráfego vai ao banco, banco fica lento, cache não consegue se repovoar porque o banco está lento.
- **Caminho de erro custoso** — tratamento de exceção mais caro que o caminho felizmais erro → mais lentidão → mais erro.
- **Balanceamento reativo** — mais tráfego para o nó que respondeu rápido por acaso.

### 9.4 Estado da arte

Bronson et al. (HotOS 2021) introduzem o termo e concluem que construir sistemas robustos a falhas metaestáveis desconhecidas permanece problema aberto. Huang et al. (OSDI 2022) documentam 22 ocorrências em 11 organizações e disponibilizam aplicações que reproduzem cada tipo. Trabalhos de 2025 (HotOS, HotNets, arXiv 2510.03551) ainda discutem a definição formal do fenômeno e as primeiras ferramentas analíticas — indício de que a área permanece aberta.

Observação relevante para a modelagem: modelos de fila padrão como M/M/c **não exibem metaestabilidade**. Identificar o que precisa ser acrescentado a eles é, em si, uma questão de pesquisa.

---

## Referências

**Dependabilidade e modelos de falta**
- Avizienis, A.; Laprie, J.-C.; Randell, B.; Landwehr, C. *Basic Concepts and Taxonomy of Dependable and Secure Computing*. IEEE TDSC, 2004.
- Fischer, M.; Lynch, N.; Paterson, M. *Impossibility of Distributed Consensus with One Faulty Process*. JACM, 1985.
- Cachin, C.; Guerraoui, R.; Rodrigues, L. *Introduction to Reliable and Secure Distributed Programming*. Springer, 2011.

**Desempenho, filas e medição**
- Harchol-Balter, M. *Performance Modeling and Design of Computer Systems: Queueing Theory in Action*. Cambridge, 2013.
- Jain, R. *The Art of Computer Systems Performance Analysis*. Wiley, 1991.
- Schroeder, B.; Wierman, A.; Harchol-Balter, M. *Open Versus Closed: A Cautionary Tale*. NSDI, 2006.
- Gregg, B. *Systems Performance*. 2ª ed., Addison-Wesley, 2020.
- Beyer, B. et al. *Site Reliability Engineering*. O'Reilly, 2016. (four golden signals)

**Chaos engineering e injeção de faltas**
- Basiri, A. et al. *Chaos Engineering*. IEEE Software, 2016.
- Alvaro, P.; Rosen, J.; Hellerstein, J. *Lineage-driven Fault Injection*. SIGMOD, 2015.
- Meiklejohn, C. et al. *Service-level Fault Injection Testing* (Filibuster). SoCC, 2021.

**Falhas metaestáveis**
- Bronson, N.; Aghayev, A.; Charapko, A.; Zhu, T. *Metastable Failures in Distributed Systems*. HotOS, 2021.
- Huang, L. et al. *Metastable Failures in the Wild*. OSDI, 2022.
- *Formal Analysis of Metastable Failures in Software Systems*. arXiv:2510.03551, 2025.

**Resiliência e complexidade**
- Perrow, C. *Normal Accidents: Living with High-Risk Technologies*. Princeton, 1984.
- Leveson, N. *Engineering a Safer World: Systems Thinking Applied to Safety*. MIT Press, 2011.
- Nygard, M. *Release It!*. 2ª ed., Pragmatic Bookshelf, 2018.
- Aderaldo, C. et al. *A declarative approach and benchmark tool for controlled evaluation of microservice resiliency patterns*. Software: Practice and Experience, 2025.

---

*Pendências: confirmar edição e paginação das referências para citação formal; verificar se o survey de padrões de recuperação (arXiv:2512.16959) tem versão publicada em veículo revisado.*