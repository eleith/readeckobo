FROM golang:1.24.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY vendor/ ./vendor/

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -a -installsuffix cgo -o /app/readeckobo ./cmd/readeckobo

FROM alpine:3.22.1

WORKDIR /app

# Install xxd for the generate-access-token script
RUN apk add --no-cache xxd util-linux

COPY --from=builder /app/readeckobo /app/readeckobo
COPY /bin/generate-token /app/bin/generate-token

ENTRYPOINT ["/app/readeckobo"]
