# Demo ao vivo (5 minutos)

Compara duas políticas de fila do servidor com **a mesma carga, o mesmo cliente e a mesma
falha**. Os números abaixo são da execução de 24/11/2026 no laptop da demo (Ubuntu, Docker
Desktop com 12 vCPU e 3,7 GB) e variam um pouco a cada vez. O que deve se repetir é o desfecho.

> **Atenção ao falar:** a demo usa uma falha de **10 s** (+200 ms em cada acesso ao banco) e
> observa 30 s de recuperação. A campanha do relatório usa **20 s** de falha e 90 s de observação.

## Antes da apresentação (cerca de 5 min, com internet)

Laptop na tomada, Docker Desktop aberto, programas pesados fechados. A partir da raiz do repositório:

```sh
cd experimento
docker info
./bancada.sh testes
./bancada.sh calibrar
```

| Comando | Duração | O que aparece | Frase |
|---|---|---|---|
| `docker info` | imediato | dados do servidor, sem erro de conexão | (só conferência) |
| `./bancada.sh testes` | ~40 s | `ok falhas-metaestaveis/servidor`, `ok falhas-metaestaveis/orquestrador` e as imagens `Built` | "Os 40 testes unitários rodam dentro do build: se um falhar, a imagem não existe." |
| `./bancada.sh calibrar` | ~3 min | `μ = 194.96 req/s … APROVADO` e `Ŝ = 41.03 ms … APROVADO` | "Antes de falar em 70 % de carga, medimos quanto é 100 % nesta máquina." |

Não rode `derrubar` depois disso: com a bancada já no ar, a demo começa uns 10 s antes.

> `calibrar` sobrescreve `resultados/calibracao.json`, que é versionado e pertence à campanha da
> máquina B. Não inclua essa mudança num commit nem retome a campanha com ela.

## Ao vivo

### 0:00–0:45 · Contexto

> "Quando o cliente desiste e tenta de novo, o pedido antigo continua na fila. Uma lentidão
> passageira pode derrubar o serviço de vez, mesmo depois que o banco volta ao normal. Isso é
> uma falha metaestável."

### 0:45–1:10 · O modelo prevê

```sh
python3 modelo-matematico/simulador.py grade
```

**Na tela (~3 s):** 4 políticas de fila × 4 cargas, com uma linha para cada cliente (`fixed` e
`backoff`). Na coluna de 70 %, `fifo` aparece `PRESO 0.0%` e `deadline`, `recuperou 100.0%`.

> "Sem usar Docker, o simulador prevê que, a 70 % de carga, a fila fixa fica presa e a fila com
> prazo se recupera. Agora vamos ver se o sistema real concorda."

A grade simula a falha de 20 s do relatório, não a de 10 s da demo.

### 1:10–2:45 · Fila fixa: fica presa

```sh
./bancada.sh cenario -fila fifo -politica fixed -carga 70 -aquecimento 5 -regime 15 -atraso 10 -recuperacao 30
```

**Na tela:** ~25 s preparando os containers, as linhas com μ e os parâmetros e depois ~65 s
**sem novas linhas de log** (é a execução). Use esse tempo para falar:

> "São 70 % da capacidade, em carga aberta: 5 s de aquecimento, 15 s normais, 10 s de banco
> lento e 30 s para ver se volta. Metade dos clientes espera 100 ms e metade espera 200 ms; cada
> um faz até 3 retries."

**Linha final:**

```
fifo-fixed-r3-c70-rep1  goodput regime 136.1/s → final 0.0/s (0 % dos pedidos novos), amplificação final 4.01, recuperação não → metaestavel
```

> "O banco voltou ao normal há 30 s e ninguém é atendido. O servidor trabalha a plena capacidade,
> mas só em pedidos já abandonados. A amplificação de 4 é o pedido original mais 3 retries."

### 2:45–4:20 · Fila com prazo: recupera

```sh
./bancada.sh cenario -fila deadline -politica fixed -carga 70 -aquecimento 5 -regime 15 -atraso 10 -recuperacao 30
```

**Durante a espera:**

> "Mesma carga, mesma falha de 10 s no banco, mesma semente de chegadas; só muda a política de
> fila. Agora o servidor descarta o pedido que já não termina dentro do prazo do cliente."

**Linha final:**

```
deadline-fixed-r3-c70-rep1  goodput regime 136.0/s → final 136.4/s (100 % dos pedidos novos), amplificação final 1.04, recuperação 0 s → recuperou
```

> "Durante a falha este também caiu. A diferença é que, assim que o banco volta, ele volta junto.
> Descartar custa quase nada, então o worker só trabalha para quem ainda está esperando."

### 4:20–5:00 · Fechamento

| Mesma carga (70 %), mesma falha (10 s) | `fifo` | `deadline` |
|---|---:|---:|
| Atendidos no prazo durante a falha | 0,0/s | 3,4/s |
| Atendidos no prazo depois da falha | 0,0/s | 136,4/s |
| Trabalho desperdiçado depois da falha | 194,0/s | 0,3/s |
| Amplificação depois da falha | 4,01 | 1,04 |
| Desfecho | **metaestavel** | **recuperou** |

> "A falha foi a mesma. O que decidiu se ela acabou foi a política de fila."

Esta é uma execução de cada política e serve de ilustração; a evidência está em
[resultados.md](resultados.md).

## Depois

```sh
./bancada.sh derrubar
```

**Na tela:** containers e rede `Removed`. Os resultados ficam em `resultados/avulso/`, e cada
nova execução sobrescreve a anterior com o mesmo nome.

## Se algo der errado

- **Docker não responde:** abra o Docker Desktop e espere `docker info` funcionar.
- **Execução muito mais longa que ~95 s:** confira a tomada e mantenha a tela ligada.
- **Desfecho diferente ou sem tempo:** mostre as linhas finais e a tabela acima, dizendo que são
  de uma execução anterior, e use a apresentação.
