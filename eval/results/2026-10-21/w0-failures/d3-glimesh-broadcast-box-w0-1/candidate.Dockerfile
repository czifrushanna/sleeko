FROM golang:alpine3.20 AS go-build
WORKDIR /broadcast-box
ENV GOPROXY=direct
ENV GOSUMDB=off
COPY . /broadcast-box
RUN apk add --no-cache git
RUN go get golang.org/x/crypto@v0.56.0 golang.org/x/net@v0.60.0 github.com/pion/interceptor@v0.1.39 github.com/pion/dtls/v3@v3.1.4 github.com/pion/stun/v3@v3.1.5 golang.org/x/sys@v0.44.0
RUN go mod tidy
RUN go build

FROM golang:alpine3.20
RUN apk add --no-cache zlib=1.3.2-r1
COPY --from=go-build /broadcast-box/broadcast-box /broadcast-box/broadcast-box
COPY --from=go-build /broadcast-box/.env.production /broadcast-box/.env.production
COPY --from=web-build /broadcast-box/web/build /broadcast-box/web/build

ENV APP_ENV=production
ENV NETWORK_TEST_ON_START=true

WORKDIR /broadcast-box
ENTRYPOINT ["/broadcast-box/broadcast-box"]
