FROM elixir:1.20-alpine

RUN apk add --no-cache bash make git

WORKDIR /app

COPY . /app

RUN make release

USER nobody

CMD _build/prod/rel/coincoin/bin/coincoin console
