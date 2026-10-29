FROM python:3.12-alpine as build

RUN apk add --no-cache build-base linux-headers libffi-dev cargo && \
    pip3 install --no-cache-dir --upgrade "pip>=26.2.0"

# Create a virtualenv that we'll copy to the published image
RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
RUN /opt/venv/bin/pip3 install --no-cache-dir "pip>=26.2.0" setuptools-rust pyopenssl cryptography

COPY . /app/
RUN cd /app/ && /opt/venv/bin/pip3 install --no-cache-dir .

# Use multi-stage build, as we don't need rust compilation on the final image
FROM python:3.12-alpine

LABEL org.opencontainers.image.documentation="https://github.com/markqvist/NomadNet#nomad-network-daemon-with-docker"

ENV PATH="/opt/venv/bin:$PATH"
ENV PYTHONUNBUFFERED="yes"

RUN apk add --no-cache "zlib>=1.3.2-r1"

COPY --from=build /opt/venv /opt/venv

VOLUME /root/.reticulum
VOLUME /root/.nomadnetwork

ENTRYPOINT ["nomadnet"]
CMD ["--daemon"]
