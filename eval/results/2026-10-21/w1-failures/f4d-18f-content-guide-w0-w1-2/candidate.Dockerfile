FROM ruby:2.7.8-bullseye

WORKDIR /usr/src/app

COPY Gemfile Gemfile.lock ./

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        imagemagick=8:6.9.11.60+dfsg-1.3+deb11u16 \
        linux-libc-dev=5.10.262-1 \
        libaom0=1.0.0.errata1-3+deb11u2 \
        perl=5.32.1-4+deb11u5 \
        python3.9=3.9.2-1+deb11u7 \
        libxml2=2.9.10+dfsg-6.7+deb11u10 \
        curl=7.74.0-1.3+deb11u16 \
        openssh-client=1:8.4p1-5+deb11u7 \
        libexpat1=2.2.10-2+deb11u7 \
        libgnutls30=3.7.1-5+deb11u10 \
        libglib2.0-0=2.66.8-1+deb11u8 \
        libheif1=1.11.0-1+deb11u2 \
        git=1:2.30.2-1+deb11u5 \
        krb5-multidev=1.18.3-6+deb11u8 \
        wget=1.21-1+deb11u2 \
        libpq-dev=13.23-0+deb11u4 \
        libssl-dev=1.1.1w-0+deb11u8 \
        libtiff-dev=4.2.0-1+deb11u8 \
        libde265-0=1.0.11-0+deb11u4 \
        libpng-dev=1.6.37-3+deb11u4 \
        bsdutils \
        libsqlite3-dev=3.34.1-3+deb11u1 \
        libc6=2.31-13+deb11u14 \
        libsystemd0=247.3-7+deb11u8 \
        libx11-dev=2:1.7.2-1+deb11u2 \
        libssh2-1=1.9.0-2+deb11u1 \
        libxslt1-dev=1.1.34-4+deb11u3 \
        libexif-dev=0.6.22-3+deb11u1 \
        gir1.2-gdkpixbuf-2.0=2.42.2+dfsg-1+deb11u5 \
        libnghttp2-14=1.43.0-1+deb11u3 \
        libncurses-dev=6.2+20201114-2+deb11u2 \
        libwebp-dev=0.6.1-2.1+deb11u2 \
        libmariadb-dev=1:10.5.29-0+deb11u1 \
        libopenjp2-7-dev=2.4.0-3+deb11u3 \
        libaprutil1=1.6.1-5+deb11u2 \
        libpam-modules=1.4.0-9+deb11u2 \
        gnupg=2.2.27-2+deb11u3 \
        libfreetype-dev=2.10.4+dfsg-1+deb11u2 \
        gir1.2-rsvg-2.0=2.50.3+dfsg-1+deb11u1 \
        libdav1d4=0.7.1-3+deb11u1 \
        liblcms2-dev=2.12~rc1-2+deb11u1 \
        comerr-dev \
        e2fsprogs=1.46.2-2+deb11u1 \
        tar=1.34+dfsg-1+deb11u1 \
        libdjvulibre-dev=3.5.28-2.2~deb11u1 \
        login=1:4.8.1-1+deb11u1 \
        libtasn1-6=4.16.0-2+deb11u2 \
        liblzma-dev=5.2.5-2.1~deb11u2 \
        file=1:5.39-3+deb11u1 \
        icu-devtools=67.1-7+deb11u1 \
        libsvn1=1.14.1-3+deb11u2 \
        libsepol1=3.1-1+deb11u1 \
        unzip=6.0-26+deb11u2 \
        dpkg=1.20.14 \
        tzdata=2026b-0+deb11u1 \
        ca-certificates=20250419~deb12u1~deb11u1 \
        mercurial=5.6.1-4+deb11u1 \
        ucf=3.0043+deb11u2 \
    && gem install concurrent-ruby -v 1.3.7 && \
    gem install stringio -v 3.0.1.1 && \
    gem install rexml -v 3.3.9 && \
    gem install bundler -v 2.2.33 && \
    gem install addressable -v 2.9.0 && \
    gem install webrick -v 1.8.2 && \
    rm -rf /var/lib/apt/lists/*

RUN bundle install --no-cache

ENV LC_ALL=C.UTF-8

CMD bundle exec jekyll serve --host 0.0.0.0 --incremental
