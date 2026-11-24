# Fechamento do Lab Exame — o que falta

Tudo está na `main`. O que está aqui é rascunho para a equipe revisar e assumir. Entregas: **apresentação final (24/nov ou 01/dez — confirmar)** e **relatório + código
até 01/dez** no Classroom.

## O que já está no repo

| Entrega | Onde | Estado |
|---|---|---|
| Código da bancada (cliente, servidor, orquestrador, análise) | `experimento/` | pronto; 40 testes passam |
| Guia do código | `experimento/CONTRIBUTING.md` | pronto |
| Reprodutibilidade em duas máquinas | `experimento/reprodutibilidade.md` | calibração + primeiras execuções; completa ao fim da campanha |
| Plano, metodologia, resultados (repetição 1) | `experimento/plano.md`, `metodologia.md`, `resultados.md` | pronto, em inglês |
| Análise matemática + simulador | `experimento/modelo-matematico/` | pronto |
| Estatística das 3 repetições | `experimento/analise/estatistica.py` | pronto, falta rodar com os dados |
| Apresentação (23 slides) | `apresentacao/apresentacao.html` (+ `.pdf`) | pronto, com 2 campos `[preencher]` |
| Relatório (itens 1–8 do anexo) | `relatorio/relatorio.md` → `relatorio.pdf` | rascunho, com campos `[PREENCHER]` |
| Decisão da mudança de foco | `docs/decisoes/0002-pivo-politicas-de-fila.md` | pronto |

## Tarefas abertas

Marque seu nome e faça PR para a `main`.

| # | Tarefa | Dono | Prazo |
|---|---|---|---|
| 1 | Confirmar a data da apresentação final | | hoje |
| 2 | Campanha de 96 execuções na máquina B (Linux): rodando; ao fim, dados em `experimento/resultados/principal/` + `estatistica.py` | Claude (em andamento) | 24/nov |
| 3 | Atualizar `resultados.md`, a seção 5.3 do relatório e os slides de resultados com as 3 repetições (média ± dp) + doc de divergências Mac × Linux | Claude, depois da campanha; Pablo atualiza os slides | |
| 3b | Rodar a mesma campanha no Mac (máquina A) com as 3 repetições, para comparar | dono do Mac | |
| 4 | Subir o **experimento 1** em `experimento-1/` (código + resultados) — o relatório se apoia nele | | |
| 5 | Rodar na bancada a fila fixa a 25, 30, 35 e 40 % (2 clientes × 3 reps, ~1 h) e comparar com a simulação (`modelo-matematico/saida_transicao.txt`) | Pablo | |
| 6 | Resolver os `[a verificar]` dos trabalhos relacionados e completar as 2 referências incompletas | | |
| 7 | Preencher **organização do grupo** (relatório §7 + último slide): atividades e % | todos | |
| 8 | Preencher **uso de IA** (relatório §8 + último slide), com honestidade, por pessoa | todos | |
| 9 | Cada um lê a fundo a parte do código pela qual responde e consegue explicá-la na arguição | todos | |
| 10 | Reescrever com palavras próprias os trechos que pareçam gerados; decidir se o relatório fica em PT ou EN | | |
| 11 | Ensaiar a apresentação (~12 min, todos falam) | todos | |
| 12 | Opcional: limite fixo com W = 48 ms, para mostrar o outro lado do trade-off | | |

## Como regenerar o relatório e os PDFs

```sh
cd relatorio
pandoc relatorio.md -s --css estilo.css --embed-resources --no-highlight -o relatorio.html
# PDF: abrir relatorio.html no navegador e imprimir em PDF (A4)
# Slides: abrir apresentacao/apresentacao.html e imprimir em PDF (o CSS já define páginas 1600×900)
```
