FROM python:3.12-alpine as build

RUN apk add --no-cache build-base linux-headers libffi-dev cargo

RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
RUN pip3 install --no-cache-dir setuptools-rust pyopenssl cryptography

COPY . /app/
RUN cd /app/ && pip3 install --no-cache-dir .

FROM python:3.12-alpine

LABEL org.opencontainers.image.documentation="https://github.com/markqvist/NomadNet#nomad-network-daemon-with-docker"

ENV PATH="/opt/venv/bin:$PATH"
ENV PYTHONUNBUFFERED="yes"

RUN apk add --no-cache zlib=1.3.2-r1

COPY --from=build /opt/venv /opt/venv

RUN pip3 install --no-cache-dir "pip>=26.2.0"

VOLUME /root/.reticulum
VOLUME /root/.nomadnetwork

ENTRYPOINT ["nomadnet"]
CMD ["--daemon"]
