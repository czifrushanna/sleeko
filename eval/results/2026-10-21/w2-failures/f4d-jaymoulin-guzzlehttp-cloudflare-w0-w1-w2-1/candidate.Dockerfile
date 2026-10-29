FROM php:7.4-alpine3.16

RUN apk add --no-cache --virtual .build-deps wget \
    && docker-php-ext-install -j$(nproc) bcmath \
    && wget https://raw.githubusercontent.com/composer/getcomposer.org/76a7060ccb93902cd7576b67264ad91c8a2700e2/web/installer -O - -q | php -- --quiet \
    && apk del --purge .build-deps
