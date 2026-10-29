FROM python:3.10.20-alpine3.22 AS builder

RUN apk add --no-cache --virtual .build-deps build-base python3-dev libffi-dev openssl-dev zlib-dev jpeg-dev libc6-compat gcc musl-dev && \
    apk upgrade --no-cache libcrypto3 libssl3 libuuid zlib musl musl-utils busybox busybox-binsh ssl_client xz-libs && \
    pip install --no-cache-dir -U "pdm>=0.1.0" "setuptools>=83.0.0" "wheel>=0.46.2" "pip>=26.2.0"

ENV PDM_CHECK_UPDATE=false
WORKDIR /app
COPY pyproject.toml README.md ./

RUN pdm install --prod --no-editable

FROM python:3.10.20-alpine3.22

WORKDIR /app

COPY --from=builder /app/.venv ./.venv

COPY xiaomusic/ ./xiaomusic/
COPY plugins/ ./plugins/
COPY holiday/ ./holiday/
COPY xiaomusic.py .

ENV PATH="/app/.venv/bin:$PATH"
