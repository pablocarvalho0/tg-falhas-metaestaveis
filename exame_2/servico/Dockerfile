FROM golang:1.25.0-alpine3.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
# Os testes rodam no build: imagem só existe se vet e testes passarem.
RUN go vet ./... && go test -count=1 ./...
RUN CGO_ENABLED=0 go build -o /out/servico .

FROM alpine:3.22
RUN apk add --no-cache wget
COPY --from=build /out/servico /usr/local/bin/servico
EXPOSE 8080
ENTRYPOINT ["servico"]
