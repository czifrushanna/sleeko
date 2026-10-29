FROM ruby:2.5-bullseye

# throw errors if Gemfile has been modified since Gemfile.lock
RUN bundle config --global frozen 1

WORKDIR /usr/src/app

COPY Gemfile Gemfile.lock ./
RUN bundle install

# Create non-root user to run the application
RUN groupadd -r jekyll && useradd -r -g jekyll jekyll
USER jekyll

EXPOSE 4000

COPY --chown=jekyll:jekyll . .

CMD ["/usr/local/bundle/bin/jekyll", "serve", "--watch", "-H", "0.0.0.0"]
