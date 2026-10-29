FROM ruby:2.3

ENV WORK_DIR /usr/lib/couchrest_model

RUN mkdir -p $WORK_DIR
WORKDIR $WORK_DIR

COPY Gemfile* ./

RUN gem install bundler -v '>= 2.2.33' --no-document && \
    gem install nokogiri -v '>= 1.19.4' --no-document && \
    gem install json -v '>= 2.3.0' --no-document && \
    gem install rake -v '>= 12.3.3' --no-document && \
    gem install kaminari -v '>= 1.2.1' --no-document && \
    gem install loofah -v '>= 2.25.2' --no-document && \
    gem install rails-html-sanitizer -v '>= 1.7.1' --no-document && \
    bundle install --jobs=3 --retry=3

COPY . $WORK_DIR

RUN bundle install --jobs=3 --retry=3
