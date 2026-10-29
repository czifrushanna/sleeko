# Base
FROM golang:1.21.4-alpine AS builder

RUN apk add --no-cache git build-base gcc musl-dev
WORKDIR /app
COPY . /app
RUN go mod download \
    && go get stdlib@1.26.9 \
    && go get golang.org/x/crypto@0.56.0 \
    && go get golang.org/x/net@0.60.0 \
    && go get golang.org/x/text@0.41.0 \
    && go get golang.org/x/oauth2@0.27.0 \
    && go get github.com/go-viper/mapstructure/v2@2.4.0 \
    && go get github.com/refraction-networking/utls@1.8.1 \
    && go get github.com/ulikunitz/xz@0.5.15 \
    && go get github.com/cloudflare/circl@1.6.3 \
    && go get golang.org/x/sys@0.44.0 \
    && go mod tidy
RUN go build ./cmd/httpx

FROM alpine:3.22
RUN apk upgrade --no-cache \
    && apk add --no-cache bind-tools ca-certificates chromium
COPY --from=builder /app/httpx /usr/local/bin/

ENTRYPOINT ["httpx"]
