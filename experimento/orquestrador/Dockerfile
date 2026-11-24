# Contexto de build: a raiz do projeto (experimento/), para incluir cliente/script.js.
FROM golang:1.25.0-alpine3.22 AS build
WORKDIR /src
COPY orquestrador/go.mod ./
COPY orquestrador/*.go ./
# Os testes rodam no build: a imagem só existe se vet e testes passarem.
RUN go vet ./... && go test -count=1 ./...
RUN CGO_ENABLED=0 go build -o /out/orquestrador .

# A imagem final é a do k6: o orquestrador roda o k6 como subprocesso.
FROM grafana/k6:1.3.0
USER root
COPY --from=build /out/orquestrador /usr/local/bin/orquestrador
COPY cliente/script.js /cliente/script.js
WORKDIR /resultados
ENTRYPOINT ["orquestrador"]
