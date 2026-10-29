FROM elixir:1.12.1-alpine

RUN apk update && apk add --no-cache bash make git

WORKDIR /app
COPY mix.exs mix.lock ./
RUN mix local.hex --force && mix deps.get

COPY . .
RUN make release

USER app
CMD _build/prod/rel/coincoin/bin/coincoin console
