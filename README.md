# Falhas metaestáveis — nota de escopo de TG

Nota de escopo para Trabalho de Graduação (ITA) sobre **falhas metaestáveis** em
sistemas distribuídos: o caso em que o mecanismo criado para proteger o sistema
(retry, cache, tratamento de erro) passa a ser o que o mantém indisponível depois
que o gatilho original já foi embora.

A página inclui um simulador interativo — um servidor de capacidade fixa sob carga,
com quatro políticas de retry — para ver o laço de realimentação fechar e, dependendo
da política, o goodput não voltar mesmo depois do fim do pico.

**Página publicada:** https://pablocarvalho0.github.io/tg-falhas-metaestaveis/

## Conteúdo

- O problema em linguagem simples (sistema aberto vs. fechado, capacidade, goodput vs. throughput)
- Os três estados: estável → vulnerável → metaestável
- Amplificadores além do retry (cache frio, tratamento de erro lento, balanceamento)
- Mitigações que atacam o laço: backoff + jitter, retry budget, circuit breaker, load shedding, propagação de deadline
- Recorte proposto, calendário e bibliografia comentada (HotOS '21, OSDI '22, arXiv 2025)

## Rodando localmente

Página única, sem build. Abra `index.html` no navegador ou sirva o diretório:

```sh
python3 -m http.server 8000
```
