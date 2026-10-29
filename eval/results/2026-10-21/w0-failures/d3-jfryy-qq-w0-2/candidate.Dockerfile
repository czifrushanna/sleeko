FROM golang:1.26.9-bullseye AS builder

WORKDIR /app
ENV CGO_ENABLED=1
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o bin/qq -ldflags="-linkmode external -extldflags -static" .

FROM gcr.io/distroless/static:nonroot
WORKDIR /qq
COPY --from=builder /app/bin/qq ./qq

ENTRYPOINT ["./qq"]
