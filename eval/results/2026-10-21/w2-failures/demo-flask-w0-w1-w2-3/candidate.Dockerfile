# syntax=docker/dockerfile:1.4
FROM --platform=$BUILDPLATFORM python:3.12-alpine3.24 AS builder

WORKDIR /app

COPY requirements.txt /app
RUN --mount=type=cache,target=/root/.cache/pip \
    pip3 install --upgrade pip setuptools wheel jaraco.context && \
    pip3 install -r requirements.txt

COPY . /app

ENTRYPOINT ["python3"]
CMD ["app.py"]

FROM builder as dev-envs

RUN apk update && \
    apk add --no-cache git && \
    addgroup -S docker && \
    adduser -S --shell /bin/bash --ingroup docker vscode && \
    apk del apk-tools && \
    rm -rf /var/cache/apk/*

COPY --from=gloursdocker/docker / /
