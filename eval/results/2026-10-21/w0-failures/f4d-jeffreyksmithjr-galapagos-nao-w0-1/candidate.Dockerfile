FROM jeffreyksmithjr/elixir-mxnet:stretch-security

COPY . .

RUN mix local.hex --force && \
    mix local.rebar --force && \
    mix deps.get && \
    mix format --check-formatted && \
    mix compile && \
    mix test

CMD iex -S mix
