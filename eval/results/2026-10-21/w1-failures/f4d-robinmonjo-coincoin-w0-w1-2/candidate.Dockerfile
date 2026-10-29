FROM elixir:1.13.4-alpine

RUN apk update && apk add --no-cache bash make git && rm -rf /var/cache/apk/*

ADD . /app
WORKDIR /app

RUN make release

USER nobody
CMD _build/prod/rel/coincoin/bin/coincoin console
