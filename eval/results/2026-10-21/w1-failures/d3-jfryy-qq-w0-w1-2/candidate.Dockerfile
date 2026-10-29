FROM golang:1.23.12-bullseye AS builder

WORKDIR /app
ENV CGO_ENABLED=1
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go get golang.org/x/net@v0.60.0 golang.org/x/text@v0.41.0 golang.org/x/sys@v0.44.0 && go mod tidy
RUN go build -o bin/qq -ldflags="-linkmode external -extldflags -static" .

FROM gcr.io/distroless/static:nonroot
WORKDIR /qq
COPY --from=builder /app/bin/qq ./qq

ENTRYPOINT ["./qq"]
