FROM debian:13-slim

RUN apt-get update && apt-get install -y --no-cache curl ca-certificates && rm -rf /var/lib/apt/lists/*

RUN adduser --disabled-password --gecos '' bruin

USER bruin

ARG VERSION=latest

RUN curl -LsSf https://raw.githubusercontent.com/bruin-data/bruin/refs/heads/main/install.sh | sh -s -- -d ${VERSION}

ENV PATH="/home/bruin/.local/bin:${PATH}"

CMD ["bruin"]
