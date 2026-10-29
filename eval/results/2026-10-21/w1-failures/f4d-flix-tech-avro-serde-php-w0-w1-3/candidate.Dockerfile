ARG PHP_VERSION=8.1

FROM php:${PHP_VERSION}-cli-alpine

ARG XDEBUG_VERSION=3.1.5

COPY --from=composer /usr/bin/composer /usr/bin/composer

RUN apk add --no-cache --virtual .build-deps $PHPIZE_DEPS \
    && apk add --no-cache --virtual .runtime-deps git libzip-dev \
    && docker-php-source extract \
    && docker-php-ext-configure zip \
    && docker-php-ext-install zip \
    && mkdir -p /usr/src/php/ext/xdebug \
    && curl -fsSL https://github.com/xdebug/xdebug/archive/$XDEBUG_VERSION.tar.gz | tar xvz -C /usr/src/php/ext/xdebug --strip 1 \
    && docker-php-ext-install xdebug \
    && docker-php-ext-enable xdebug \
    && docker-php-source delete \
    && apk del .build-deps \
    && apk add --no-cache \
        openssl=3.3.7-r2 \
        libcrypto3=3.3.7-r2 \
        libssl3=3.3.7-r2 \
        curl=8.14.1-r3 \
        libcurl=8.14.1-r3 \
        musl-utils=1.2.5-r11 \
        c-ares=1.34.8-r0 \
        libxml2=2.13.9-r1 \
        nghttp2-libs=1.68.1 \
        busybox=1.37.0-r14 \
        busybox-binsh=1.37.0-r14 \
        ssl_client=1.37.0-r14 \
        libsodium=1.0.20-r1 \
    && rm -rf /var/cache/apk/*

RUN composer --version
