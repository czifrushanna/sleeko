FROM golang:1.23-alpine AS builder
WORKDIR /DNSlog-GO
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV GOPROXY https://goproxy.cn,direct
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o main main.go

FROM alpine:3.20 AS runner
WORKDIR /DNSlog-GO
COPY --from=builder /DNSlog-GO/main .
COPY --from=builder /DNSlog-GO/config.yaml .
RUN apk add --no-cache tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo Asia/Shanghai > /etc/timezone \
    && apk del tzdata
EXPOSE 53/udp 8000
ENTRYPOINT ["./main"]
