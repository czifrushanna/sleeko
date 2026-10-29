FROM debian:stretch-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        gnupg2 \
        libcurl4-openssl-dev \
        libgmp-dev \
        libopenblas-dev \
        liblapack-dev \
        libblas-dev \
        libatlas-base-dev \
        libpng-dev \
        libjpeg-dev \
        libtiff-dev \
        libwebp-dev \
        libfreetype6-dev \
        libz-dev \
        zlib1g-dev \
        build-essential \
        cmake \
        m4 \
        autoconf \
        automake \
        libtool \
        pkg-config \
        python3 \
        python3-dev \
        python3-pip \
        python3-setuptools \
        wget \
        unzip \
        ca-certificates-java \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://keybase.io/elixir.asc | gpg --import && \
    curl -fsSL https://keybase.io/erlang.asc | gpg --import && \
    apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 277177F1B5811458C0B13F8DF4B07E0A862C956D && \
    echo "deb https://packages.erlang-solutions.com/debian/ stretch main" > /etc/apt/sources.list.d/erlang-solutions.list && \
    echo "deb https://repo.hex.pm/builds/debian/ stretch main" > /etc/apt/sources.list.d/hex.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends erlang && \
    apt-get install -y --no-install-recommends elixir && \
    rm -rf /var/lib/apt/lists/*

RUN mix local.hex --force && \
    mix local.rebar --force

COPY mix.exs mix.lock ./
RUN mix deps.get && \
    mix deps.compile

COPY . .
RUN mix format --check-formatted && \
    mix compile --warnings-as-errors

CMD ["iex", "-S", "mix"]
