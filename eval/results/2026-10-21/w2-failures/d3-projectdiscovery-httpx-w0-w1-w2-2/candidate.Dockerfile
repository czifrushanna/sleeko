FROM golang:1.21.4-alpine AS builder

RUN apk add --no-cache git build-base gcc musl-dev
WORKDIR /app
COPY . /app
RUN go mod edit -require=golang.org/x/crypto@v0.56.0 -require=golang.org/x/net@v0.60.0 -require=golang.org/x/text@v0.41.0 -require=golang.org/x/oauth2@v0.27.0 -require=github.com/go-viper/mapstructure/v2@v2.4.0 -require=github.com/refraction-networking/utls@v1.8.1 -require=github.com/ulikunitz/xz@v0.5.15 -require=github.com/cloudflare/circl@v1.6.3 -require=golang.org/x/sys@v0.44.0 && go mod tidy && go mod download
RUN go build ./cmd/httpx

FROM alpine:3.22
RUN apk upgrade --no-cache \
    && apk add --no-cache bind-tools ca-certificates chromium
COPY --from=builder /app/httpx /usr/local/bin/

ENTRYPOINT ["httpx"]
