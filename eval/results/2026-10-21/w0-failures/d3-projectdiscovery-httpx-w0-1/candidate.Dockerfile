FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git build-base gcc musl-dev
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /app/httpx ./cmd/httpx

FROM alpine:3.20

RUN apk add --no-cache bind-tools ca-certificates chromium && \
    addgroup -S appgroup && \
    adduser -S appuser -G appgroup

COPY --from=builder /app/httpx /usr/local/bin/

USER appuser

ENTRYPOINT ["httpx"]
