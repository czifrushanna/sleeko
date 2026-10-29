FROM elixir:1.13-alpine

RUN apk update && apk add --no-cache bash make git && \
    apk upgrade --no-cache

WORKDIR /app

ADD . /app
RUN make release

USER nobody

CMD _build/prod/rel/coincoin/bin/coincoin console
