# syntax=docker/dockerfile:1.4
FROM --platform=$BUILDPLATFORM python:3.10-alpine AS builder

WORKDIR /app

COPY requirements.txt /app
RUN --mount=type=cache,target=/root/.cache/pip \
    pip3 install --upgrade pip wheel setuptools jaraco.context -r requirements.txt

COPY . /app

ENTRYPOINT ["python3"]
CMD ["app.py"]

FROM python:3.12-alpine3.24 as dev-envs

WORKDIR /app

RUN <<EOF
apk update
apk add git
apk del zlib
apk add zlib
EOF

RUN <<EOF
addgroup -S docker
adduser -S --shell /bin/bash --ingroup docker vscode
EOF
# install Docker tools (cli, buildx, compose)
COPY --from=gloursdocker/docker / /

ENTRYPOINT ["python3"]
CMD ["app.py"]
