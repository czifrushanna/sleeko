FROM ruby:2.5-slim

ENV WORK_DIR /usr/lib/couchrest_model

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libpq-dev \
    libmysqlclient-dev \
    libsqlite3-dev \
    libxml2-dev \
    libxslt1-dev \
    zlib1g-dev \
    libyaml-dev \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR $WORK_DIR

COPY Gemfile Gemfile.lock ./

RUN gem update --system && \
    gem install bundler -v '>= 2.2.33' && \
    bundle install --jobs=3 --retry=3 --without development test

COPY . .

USER 1001

CMD ["bundle", "exec", "rake"]
