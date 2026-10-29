FROM php:7.1-cli-alpine3.12

RUN apk add --no-cache git composer

COPY . /app

WORKDIR /app
