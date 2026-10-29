#
FROM golang:1.21.4-alpine AS builder

RUN apk add --no-cache git build-base gcc musl-dev
WORKDIR /app
COPY . /app
RUN go get golang.org/x/crypto@0.56.0 golang.org/x/net@0.60.0 golang.org/x/text@0.41.0 golang.org/x/oauth2@0.27.0 github.com/go-viper/mapstructure/v2@2.4.0 github.com/refraction-networking/utls@1.8.1 github.com/ulikunitz/xz@0.5.15 github.com/cloudflare/circl@1.6.3 golang.org/x/sys@0.44.0
RUN go mod download
RUN go build ./cmd/httpx

FROM alpine:3.22
RUN apk upgrade --no-cache \
    && apk add --no-cache bind-tools ca-certificates chromium
COPY --from=builder /app/httpx /usr/local/bin/

ENTRYPOINT ["httpx"]
