FROM ruby:2.5-slim

ENV WORK_DIR /usr/lib/couchrest_model

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential=12.1.0-1ubuntu1 \
    libpq-dev=10.22-0ubuntu0.18.04.1 \
    libmysqlclient-dev=5.7.33-0ubuntu0.18.04.1 \
    libsqlite3-dev=3.22.0-1ubuntu0.1 \
    libxml2-dev=2.9.4+dfsg1-6.1ubuntu1.5 \
    libxslt1-dev=1.1.29+dfsg-1.1 \
    zlib1g-dev=1:1.2.11.dfsg-0ubuntu2.4 \
    libyaml-dev=0.1.8-3ubuntu0.1 \
    git=1:2.17.1-1ubuntu0.15 \
    curl=7.58.0-2ubuntu3.20 \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p $WORK_DIR
WORKDIR $WORK_DIR

COPY Gemfile Gemfile.lock ./

RUN gem update --system && \
    gem install bundler -v '>= 2.2.33' && \
    bundle install --jobs=3 --retry=3 --without development test

COPY . .

USER 1001
