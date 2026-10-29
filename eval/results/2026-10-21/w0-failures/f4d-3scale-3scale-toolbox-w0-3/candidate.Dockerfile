FROM registry.access.redhat.com/ubi9/ruby-30:latest

MAINTAINER Eguzki Astiz Lezaun <eastizle@redhat.com>

USER root

RUN dnf upgrade --security -y environment-modules && \
    dnf clean all && \
    gem install bundler --version 2.3.5 --no-document && \
    gem install nokogiri -v '>= 1.19.4' --no-document && \
    gem install concurrent-ruby -v '>= 1.3.7' --no-document && \
    gem install addressable -v '>= 2.9.0' --no-document && \
    chmod +t /tmp

WORKDIR /usr/src/app

COPY . .

RUN bundle config --local silence_root_warning 1 \
    && bundle config --local disable_shared_gems 1 \
    && bundle config --local without "development test" \
    && bundle config set --local deployment 'true' \
    && bundle config --local gemfile Gemfile \
    && bundle config set --local path 'vendor/bundle' \
    && bundle install \
    && bundle binstubs 3scale_toolbox

ENV PATH="/usr/src/app/bin:${PATH}"

# Drop privileges
USER default

WORKDIR /opt/app-root/src

CMD ["/bin/bash"]
