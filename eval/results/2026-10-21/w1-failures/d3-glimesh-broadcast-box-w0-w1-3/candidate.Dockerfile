FROM golang:1.25.0-alpine3.21 AS go-build
WORKDIR /broadcast-box
ENV GOPROXY=direct
ENV GOSUMDB=off
COPY . /broadcast-box
RUN apk add --no-cache git \
    && go get golang.org/x/crypto@v0.56.0 \
    && go get golang.org/x/net@v0.60.0 \
    && go get github.com/pion/interceptor@v0.1.39 \
    && go get github.com/pion/dtls/v3@v3.1.4 \
    && go get github.com/pion/stun/v3@v3.1.5 \
    && go get golang.org/x/sys@v0.44.0 \
    && go mod tidy \
    && go build -o broadcast-box . \
    && apk del git

FROM node:22-alpine AS web-build
WORKDIR /broadcast-box/web
COPY . /broadcast-box
RUN npm install --omit=dev && npm run build

FROM alpine:3.21
RUN apk add --no-cache zlib
COPY --from=web-build /broadcast-box/web/build /broadcast-box/web/build
COPY --from=go-build /broadcast-box/broadcast-box /broadcast-box/broadcast-box
COPY --from=go-build /broadcast-box/.env.production /broadcast-box/.env.production

ENV APP_ENV=production
ENV NETWORK_TEST_ON_START=true

WORKDIR /broadcast-box
ENTRYPOINT ["/broadcast-box/broadcast-box"]
