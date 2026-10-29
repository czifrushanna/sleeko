FROM ruby:2.5-buster

# throw errors if Gemfile has been modified since Gemfile.lock
RUN bundle config --global frozen 1

WORKDIR /usr/src/app

COPY Gemfile Gemfile.lock ./
RUN gem install bundler -v '>= 2.2.33' --no-document && \
    bundle install

RUN jekyll build

EXPOSE 4000

COPY . .

CMD ["/usr/local/bundle/bin/jekyll", "serve", "--watch", "-H", "0.0.0.0"]
