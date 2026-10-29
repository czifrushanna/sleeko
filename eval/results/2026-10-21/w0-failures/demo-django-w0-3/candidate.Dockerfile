# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM python:3.12-alpine AS builder
WORKDIR /app
COPY requirements.txt /app
RUN pip3 install --no-cache-dir -r requirements.txt
COPY . /app

FROM python:3.12-alpine AS dev-envs
WORKDIR /app
COPY --from=builder /app /app
COPY --from=builder /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin

RUN apk update && \
    apk add --no-cache git && \
    addgroup -S docker && \
    adduser -S --shell /bin/bash --ingroup docker vscode && \
    rm -rf /var/cache/apk/*

COPY --from=gloursdocker/docker / /
USER vscode
EXPOSE 8000
ENTRYPOINT ["python3"]
CMD ["manage.py", "runserver", "0.0.0.0:8000"]
