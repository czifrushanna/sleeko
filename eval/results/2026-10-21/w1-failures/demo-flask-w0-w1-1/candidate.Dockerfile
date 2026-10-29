# syntax=docker/dockerfile:1.4
FROM --platform=$BUILDPLATFORM python:3.10-alpine AS builder

WORKDIR /app

RUN apk update && apk add --no-cache zlib && \
    pip install --no-cache-dir --upgrade "pip>=26.2.0" "setuptools>=83.0.0" "wheel>=0.46.2" "jaraco.context>=6.1.0" && \
    apk del zlib && apk cache clean

COPY requirements.txt /app
RUN --mount=type=cache,target=/root/.cache/pip \
    pip3 install -r requirements.txt

COPY . /app

ENTRYPOINT ["python3"]
CMD ["app.py"]

FROM builder as dev-envs

RUN apk update && \
    apk add --no-cache git && \
    addgroup -S docker && \
    adduser -S --shell /bin/bash --ingroup docker vscode && \
    apk del git && \
    apk cache clean

COPY --from=gloursdocker/docker / /
