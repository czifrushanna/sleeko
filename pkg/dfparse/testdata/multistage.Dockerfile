# syntax=docker/dockerfile:1
ARG GO_VERSION=1.22
ARG BUILDER=golang:${GO_VERSION}-bookworm
ARG RUNTIME_TAG

FROM --platform=$BUILDPLATFORM ${BUILDER} AS Build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM build AS test
RUN go test ./...

FROM debian:bookworm-slim AS runtime-base
RUN apt-get update && apt-get install -y tzdata && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["--help"]
EXPOSE 9090

FROM gcr.io/distroless/static-debian12:${RUNTIME_TAG} AS unused

FROM runtime-base
COPY --from=build /out/server /server
COPY --from=alpine:3.20 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/server"]
EXPOSE 8080 9090
USER 65532:65532
