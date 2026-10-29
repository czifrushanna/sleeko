FROM php:7.1-cli-alpine3.13

RUN apk add --no-cache git

COPY --from=composer /usr/bin/composer /usr/bin/composer
