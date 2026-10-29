FROM docker.io/library/ruby:3.2-alpine AS build-image

RUN apk update && \
    apk add --no-cache ruby-bundler git build-base gcc abuild binutils linux-headers gmp-dev

ADD Gemfile Gemfile.lock /
RUN gem install -v 2.3.20 bundler && bundle install

FROM docker.io/library/ruby:3.2-alpine

ENV APP_HOME=/opt/moarcats
ENV RACK_ENV=production
ENV PORT=5000
ENV CATS_DIR=/cats

EXPOSE $PORT

COPY --from=build-image /usr/local/bundle /usr/local/bundle
COPY --from=build-image /Gemfile.lock $APP_HOME/Gemfile.lock

WORKDIR $APP_HOME
VOLUME /cats
ADD Gemfile $APP_HOME
COPY . $APP_HOME

USER nobody
CMD ["rake", "puma:start"]
