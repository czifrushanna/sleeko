FROM python:3.12-alpine as build

RUN apk add --no-cache build-base linux-headers libffi-dev cargo

RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

RUN pip3 install --upgrade pip==26.2.0 && \
    pip3 install setuptools-rust pyopenssl cryptography

COPY . /app/
RUN cd /app/ && pip3 install .

FROM python:3.12-alpine

RUN apk add --no-cache --upgrade zlib

LABEL org.opencontainers.image.documentation="https://github.com/markqvist/NomadNet#nomad-network-daemon-with-docker"

ENV PATH="/opt/venv/bin:$PATH"
ENV PYTHONUNBUFFERED="yes"

COPY --from=build /opt/venv /opt/venv

RUN addgroup -S appgroup && adduser -S appuser -G appgroup && \
    chown -R appuser:appgroup /opt/venv && \
    mkdir -p /home/appuser/.reticulum /home/appuser/.nomadnetwork && \
    chown -R appuser:appgroup /home/appuser

USER appuser

VOLUME /home/appuser/.reticulum
VOLUME /home/appuser/.nomadnetwork

ENTRYPOINT ["nomadnet"]
CMD ["--daemon"]
