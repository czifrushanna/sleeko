# syntax = docker/dockerfile:1.4

FROM python:3.9-slim-bookworm AS builder

WORKDIR /app

COPY requirements.txt ./
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-cache-dir -r requirements.txt

COPY ./app ./app

FROM python:3.9-slim-bookworm AS dev-envs

RUN <<EOF
set -eux
apt-get update
apt-get install -y --no-install-recommends git ca-certificates
apt-get clean
rm -rf /var/lib/apt/lists/*
useradd -s /bin/bash -m vscode
groupadd docker
usermod -aG docker vscode
EOF

COPY --from=gloursdocker/docker / /

COPY --from=builder /usr/local/lib/python3.9/site-packages /usr/local/lib/python3.9/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin
COPY --from=builder /app /app

USER vscode
