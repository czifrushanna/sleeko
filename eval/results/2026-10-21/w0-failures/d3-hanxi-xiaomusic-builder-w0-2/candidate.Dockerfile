FROM python:3.10.18-alpine3.22 AS builder

RUN apk add --no-cache --virtual .build-deps build-base python3-dev libffi-dev openssl-dev zlib-dev jpeg-dev libc6-compat gcc musl-dev && \
    apk add --no-cache libcrypto3=3.5.8-r0 libssl3=3.5.8-r0 libuuid=2.41.6-r1 zlib=1.3.2-r1 musl=1.2.5-r12 musl-utils=1.2.5-r12 busybox=1.37.0-r20 busybox-binsh=1.37.0-r20 ssl_client=1.37.0-r20 xz-libs=5.8.3-r0

RUN pip install --no-cache-dir -U pdm setuptools==83.0.0 wheel==0.46.2 pip==26.2.0

ENV PDM_CHECK_UPDATE=false
WORKDIR /app
COPY pyproject.toml README.md ./

RUN pdm install --prod --no-editable

FROM python:3.10.18-alpine3.22

RUN apk add --no-cache libcrypto3=3.5.8-r0 libssl3=3.5.8-r0 libuuid=2.41.6-r1 zlib=1.3.2-r1 musl=1.2.5-r12 musl-utils=1.2.5-r12 busybox=1.37.0-r20 busybox-binsh=1.37.0-r20 ssl_client=1.37.0-r20 xz-libs=5.8.3-r0

WORKDIR /app

COPY --from=builder /app/.venv ./.venv

COPY xiaomusic/ ./xiaomusic/
COPY plugins/ ./plugins/
COPY holiday/ ./holiday/
COPY xiaomusic.py .
