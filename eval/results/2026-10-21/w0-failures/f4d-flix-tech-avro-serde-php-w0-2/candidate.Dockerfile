ARG PHP_VERSION=8.1

FROM php:${PHP_VERSION}-cli-alpine:3.20

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
    && apk upgrade --no-cache libcrypto3 libssl3 openssl curl libcurl musl-utils c-ares libxml2 nghttp2-libs busybox busybox-binsh ssl_client libsodium

RUN composer --version
