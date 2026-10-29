# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM python:3.12-alpine AS builder
WORKDIR /app

COPY requirements.txt /app
RUN pip3 install --no-cache-dir -r requirements.txt

COPY . /app

ENTRYPOINT ["python3"]
CMD ["manage.py", "runserver", "0.0.0.0:8000"]

FROM builder AS dev-envs
RUN apk add --no-cache git
RUN addgroup -S docker && adduser -S --shell /bin/bash --ingroup docker vscode
COPY --from=gloursdocker/docker / /
CMD ["manage.py", "runserver", "0.0.0.0:8000"]
