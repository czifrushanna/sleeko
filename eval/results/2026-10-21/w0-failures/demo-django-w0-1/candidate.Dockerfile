# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM python:3.12-alpine AS builder
WORKDIR /app

COPY requirements.txt /app
RUN pip install --upgrade pip==26.2.0 setuptools==83.0.0 wheel==0.46.2
RUN pip install --no-cache-dir \
    "Django>=5.2.17,<6.0" \
    "sqlparse>=0.6.0" \
    "marshmallow>=3.26.2,<4.0" \
    "python-dotenv>=1.2.2" \
    -r requirements.txt

COPY . /app

FROM alpine:3.20

RUN apk add --no-cache \
    python3 \
    py3-pip \
    git \
    && addgroup -S docker \
    && adduser -S --shell /bin/bash --ingroup docker vscode

COPY --from=builder /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin
COPY --from=builder /app /app

# Install Docker tools (cli, buildx, compose)
COPY --from=gloursdocker/docker / /

WORKDIR /app
USER vscode
EXPOSE 8000

ENTRYPOINT ["python3"]
CMD ["manage.py", "runserver", "0.0.0.0:8000"]
