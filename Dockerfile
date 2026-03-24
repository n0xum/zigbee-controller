FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bridge ./cmd/bridge

FROM alpine:3.21

RUN adduser -D appuser && \
    mkdir -p /app/hap-data && \
    chown appuser /app/hap-data

WORKDIR /app
COPY --from=builder /build/bridge /app/bridge

USER appuser

ENTRYPOINT ["/app/bridge"]
