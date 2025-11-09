FROM golang:1.24.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY vendor/ ./vendor/

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -a -installsuffix cgo -o /app/readeckobo ./cmd/readeckobo

FROM alpine:3.22.1

WORKDIR /app

# Install tools for the token generation script
RUN apk add --no-cache xxd util-linux openssl coreutils

COPY --from=builder /app/readeckobo /app/readeckobo
COPY /bin/generate-encrypted-token.sh /app/bin/generate-encrypted-token.sh

ENTRYPOINT ["/app/readeckobo"]
