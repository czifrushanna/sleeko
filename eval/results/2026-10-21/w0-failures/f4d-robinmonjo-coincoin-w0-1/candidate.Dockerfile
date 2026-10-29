FROM elixir:1.12.1-alpine3.12

RUN apk update && apk add --no-cache bash make git

COPY mix.exs mix.lock ./
RUN mix local.hex --force && mix deps.get

COPY . .
RUN mix deps.compile && mix release

USER app
WORKDIR /app
CMD _build/prod/rel/coincoin/bin/coincoin console
