FROM golang:1.26-alpine AS builder

# `apk add --no-cache` já atualiza os índices que precisa. NÃO encadear com `apk update`
# separado: `apk update` falha (exit != 0) se QUALQUER repositório configurado estiver
# fora do ar — inclusive `community`, que não usamos aqui —, e o `&&` derrubava o build
# inteiro mesmo com os pacotes abaixo (todos em `main`) disponíveis.
#
# `git` NÃO entra aqui: o go.mod deste projeto não tem `replace` para VCS e todo módulo é
# público, então `go mod download` resolve tudo via proxy.golang.org (HTTPS), sem precisar
# de git. Era peso morto — e era também o pacote mais pesado (mais arquivos a extrair),
# exatamente o que falhou com "I/O error" na extração num build anterior.
#
# Retry (3x) só para absorver uma extração pontualmente corrompida/truncada — não mascara
# falha real: o `exit 1` final só é alcançado se as 3 tentativas falharem (sem ele, o
# status do RUN seria o do último comando do loop, "sleep", que sempre é 0 — sucesso falso
# mesmo com o apk add nunca tendo funcionado).
RUN for i in 1 2 3; do \
      apk add --no-cache ca-certificates tzdata && exit 0; \
      echo "apk add falhou (tentativa $i/3), tentando de novo em 3s..." >&2; \
      sleep 3; \
    done; \
    exit 1

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o main ./cmd/api/main.go

FROM alpine:3.18

ENV GIN_MODE=release

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]
