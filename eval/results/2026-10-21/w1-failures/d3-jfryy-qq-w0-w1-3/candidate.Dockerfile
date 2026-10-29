FROM golang:1.23-bullseye AS builder

WORKDIR /app
COPY . .
ENV CGO_ENABLED=1
RUN go get golang.org/x/net@v0.60.0 golang.org/x/text@v0.41.0 golang.org/x/sys@v0.44.0 && go mod tidy && go build -o bin/qq -ldflags="-linkmode external -extldflags -static" .

FROM gcr.io/distroless/static:nonroot
WORKDIR /qq
COPY --from=builder /app/bin/qq ./qq

ENTRYPOINT ["./qq"]
