FROM elixir:1.20-alpine

RUN apk add --no-cache bash make git

COPY . /app
WORKDIR /app
RUN make release

CMD _build/prod/rel/coincoin/bin/coincoin console
