# Engineering Resilient Distributed Systems — nota de escopo de TG

### → [Ler a nota (com fluxogramas e simulador interativo)](https://pablocarvalho0.github.io/tg-falhas-metaestaveis/)

[![Publicar página](https://github.com/pablocarvalho0/tg-falhas-metaestaveis/actions/workflows/pages.yml/badge.svg)](https://github.com/pablocarvalho0/tg-falhas-metaestaveis/actions/workflows/pages.yml)

A página é republicada a cada push na `main` — o link acima serve sempre o
`index.html` do último commit. O badge mostra se a última publicação passou.

Nota de escopo para Trabalho de Graduação (ITA), a partir do rascunho da Prof.ª
Juliana de Melo Bezerra de 25/08/2026. O trabalho é uma **esteira de ponta a ponta
para medir como um serviço responde a estresse** — modelo de falha, modelo de carga,
cenário, observação e mitigação — percorrida em espiral sobre um sistema de exemplo,
com um alvo no fim: reproduzir em bancada uma **falha metaestável**, o caso em que o
sistema não volta sozinho depois que o gatilho já foi embora.

A página traz dois fluxogramas (a esteira com o laço da espiral, e a topologia da
bancada com os pontos de injeção de falha) e um simulador interativo — um servidor de
capacidade fixa sob carga, com quatro políticas de retry — para ver o laço de
realimentação fechar.

## Conteúdo

- **A esteira**: as cinco etapas do rascunho lidas como um método único, cada uma com
  ferramenta, decisão a tomar e a armadilha correspondente
- **O fenômeno**: simulador interativo e a teoria mínima (sistema aberto vs. fechado,
  capacidade, goodput vs. throughput, os três estados, amplificadores e mitigações)
- **A bancada**: sistema de exemplo com quatro containers e um proxy, gargalo
  deliberado via pool fixo, e por que não construir do zero no primeiro semestre
- **Escopo**: etapas necessárias e desejáveis separadas, camada analítica e calendário
- **Leituras**: bibliografia comentada (HotOS '21, OSDI '22, NSDI '06, IEEE TDSC,
  IEEE Software, arXiv 2025) e o alerta sobre periódicos predatórios
- **Reunião**: as perguntas em aberto para fechar escopo

## Rodando localmente

Página única, sem build. Abra `index.html` no navegador ou sirva o diretório:

```sh
python3 -m http.server 8000
```
