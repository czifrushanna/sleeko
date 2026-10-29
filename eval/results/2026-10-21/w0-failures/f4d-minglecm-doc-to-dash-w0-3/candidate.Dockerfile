FROM ruby:3.4.1-slim-bookworm

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        curl=8.14.1-2+deb13u7 \
        libcurl3t64-gnutls=8.14.1-2+deb13u7 \
        libcurl4-openssl-dev=8.14.1-2+deb13u7 \
        libcurl4t64=8.14.1-2+deb13u7 \
        libexpat1=2.9.0-1~deb13u1 \
        libexpat1-dev=2.9.0-1~deb13u1 \
        gir1.2-rsvg-2.0=2.60.0+dfsg-1+deb13u1 \
        librsvg2-2=2.60.0+dfsg-1+deb13u1 \
        librsvg2-common=2.60.0+dfsg-1+deb13u1 \
        librsvg2-dev=2.60.0+dfsg-1+deb13u1 \
        liblzma-dev=5.8.1-1+deb13u2 \
        liblzma5=5.8.1-1+deb13u2 \
        xz-utils=5.8.1-1+deb13u2 \
        linux-libc-dev=6.12.113-1 && \
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
