FROM elixir:1.20-alpine

RUN apk update && apk add --no-cache bash make git

ADD . /app
WORKDIR /app
RUN make release

RUN addgroup -S appgroup && adduser -S appuser -G appgroup && chown -R appuser:appgroup /app
USER appuser

CMD _build/prod/rel/coincoin/bin/coincoin console
