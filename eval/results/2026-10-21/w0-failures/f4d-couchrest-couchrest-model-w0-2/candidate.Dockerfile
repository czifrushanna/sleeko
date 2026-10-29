FROM ruby:2.3.9-slim

ENV WORK_DIR /usr/lib/couchrest_model

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        build-essential \
        libpq-dev \
        libsqlite3-dev \
        libxml2-dev \
        libxslt1-dev \
        zlib1g-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR $WORK_DIR

COPY Gemfile Gemfile.lock ./
RUN gem install bundler -v '~> 2.2.33' && \
    bundle install --jobs=3 --retry=3

COPY . .

CMD ["bundle", "exec", "rake"]
