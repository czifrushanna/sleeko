FROM php:7.1-cli-alpine3.12

RUN apk add --no-cache git=2.25.1-r0
COPY --from=composer /usr/bin/composer /usr/bin/composer
