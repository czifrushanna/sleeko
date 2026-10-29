FROM php:7.1-cli-alpine

RUN apk add --no-cache \
    git \
    curl=7.66.0-r4 \
    libcurl=7.66.0-r4 \
    libssl1.1=1.1.1k-r0 \
    libcrypto1.1=1.1.1k-r0 \
    openssl=1.1.1k-r0 \
    libxml2=2.9.9-r5 \
    sqlite-libs=3.28.0-r3 \
    busybox=1.30.1-r5 \
    nghttp2-libs=1.39.2-r1 \
    ssl_client=1.30.1-r5 \
    musl=1.1.22-r4 \
    musl-utils=1.1.22-r4 \
    tar=1.32-r1 \
    apk-tools=2.10.7-r0

COPY --from=composer /usr/bin/composer /usr/bin/composer
