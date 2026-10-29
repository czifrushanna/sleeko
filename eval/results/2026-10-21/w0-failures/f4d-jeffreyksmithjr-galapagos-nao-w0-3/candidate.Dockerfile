FROM jeffreyksmithjr/elixir-mxnet

USER root

RUN apt-get update && apt-get -y upgrade --no-install-recommends && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY . .

RUN mix local.hex --force && \
    mix local.rebar --force && \
    mix deps.get && \
    mix format --check-formatted && \
    mix compile

RUN mix test

CMD iex -S mix
