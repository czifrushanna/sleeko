FROM debian:stretch-slim

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates=20200601~deb9u2 \
        curl=7.52.1-5+deb9u16 \
        git=1:2.11.0-3+deb9u7 \
        gnupg2=2.1.18-8~deb9u2 \
        libcurl4-openssl-dev=7.52.1-5+deb9u16 \
        libgmp-dev=2:6.1.2+dfsg-1+deb9u1 \
        libopenblas-dev \
        liblapack-dev=3.7.0-2 \
        libblas-dev=3.7.0-2 \
        libatlas-base-dev \
        libpng-dev=1.6.28-1+deb9u1 \
        libjpeg-dev=1:1.5.1-2+deb9u2 \
        libtiff-dev \
        libwebp-dev=0.5.2-1+deb9u1 \
        libfreetype6-dev=2.6.3-3.2+deb9u2 \
        libzbar0 \
        libhdf5-serial-dev \
        libssl-dev=1.1.0l-1~deb9u6 \
        zlib1g-dev=1:1.2.8.dfsg-5+deb9u1 \
        build-essential \
        cmake=3.7.2-1 \
        gcc \
        g++ \
        make=4.1-9.1 \
        pkg-config \
        python3 \
        python3-pip \
        python3-dev \
        wget=1.18-5+deb9u3 \
        unzip \
        lsb-release \
    && rm -rf /var/lib/apt/lists/*

RUN curl -sSfL https://get.erlang.org/package-scripts/erlang_solutions_package_install.sh | sh \
    && apt-get update \
    && apt-get install -y --no-install-recommends erlang-nox \
    && rm -rf /var/lib/apt/lists/*

RUN curl -sSfL https://raw.githubusercontent.com/elixir-lang/elixir/v1.13.4/install.sh | bash \
    && ln -s /usr/local/bin/elixir /usr/bin/elixir \
    && ln -s /usr/local/bin/iex /usr/bin/iex \
    && ln -s /usr/local/bin/mix /usr/bin/mix

RUN pip3 install --no-cache-dir mxnet==1.9.1

COPY . .

RUN mix local.hex --force \
    && mix local.rebar --force \
    && mix deps.get \
    && mix format --check-formatted \
    && mix compile \
    && mix test

CMD iex -S mix
