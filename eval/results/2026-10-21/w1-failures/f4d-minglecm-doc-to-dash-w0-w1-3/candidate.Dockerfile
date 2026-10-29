FROM ruby:3.4.1-slim-bookworm

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        curl \
        libcurl3t64-gnutls \
        libcurl4-openssl-dev \
        libcurl4t64 \
        libexpat1 \
        libexpat1-dev \
        gir1.2-rsvg-2.0 \
        librsvg2-2 \
        librsvg2-common \
        librsvg2-dev \
        liblzma-dev \
        liblzma5 \
        xz-utils \
        linux-libc-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    touch ~/.gemrc && \
    echo "gem: --no-ri --no-rdoc" >> ~/.gemrc && \
    mkdir -p /gem/

WORKDIR /gem/

COPY . /gem/

RUN bundle install

VOLUME .:/gem/

ENTRYPOINT ["bundle", "exec"]
CMD ["rake", "-T"]
