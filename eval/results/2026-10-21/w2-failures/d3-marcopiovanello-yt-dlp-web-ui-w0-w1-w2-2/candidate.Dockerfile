# Node (pnpm) ------------------------------------------------------------------
FROM node:22-slim AS ui
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack prepare pnpm@10.0.0 --activate && corepack enable
COPY . /usr/src/yt-dlp-webui

WORKDIR /usr/src/yt-dlp-webui/frontend

RUN rm -rf node_modules && \
    pnpm install && \
    pnpm run build
# -----------------------------------------------------------------------------

# Go --------------------------------------------------------------------------
FROM golang AS build

WORKDIR /usr/src/yt-dlp-webui

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=ui /usr/src/yt-dlp-webui/frontend /usr/src/yt-dlp-webui/frontend

RUN CGO_ENABLED=0 GOOS=linux go build -o yt-dlp-webui
# -----------------------------------------------------------------------------

# Runtime ---------------------------------------------------------------------
FROM python:3.13-alpine3.24

RUN apk add --no-cache ffmpeg ca-certificates curl wget gnutls && \
    pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir "yt-dlp[default,curl-cffi,mutagen,pycryptodomex,phantomjs,secretstorage]"

VOLUME /downloads /config

WORKDIR /app

COPY --from=build /usr/src/yt-dlp-webui/yt-dlp-webui /app

ENV JWT_SECRET=secret

EXPOSE 3033
ENTRYPOINT [ "./yt-dlp-webui" , "--out", "/downloads", "--conf", "/config/config.yml", "--db", "/config/local.db" ]
