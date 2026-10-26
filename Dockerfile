# Build the sleeko optimizer adapter service.
#
#   docker build -t sleeko:dev .
#
ARG golang_image=golang:1.26.4

FROM ${golang_image} AS build

ARG version=dev

WORKDIR /sleeko
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/czifrushanna/sleeko/pkg/adapter.Version=${version}" \
    -o /sleeko-bin ./cmd/sleeko

# The adapter calls the LLM gateway over HTTPS, so CA certificates are required.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /sleeko-bin /sleeko

EXPOSE 8080
USER 10000
ENTRYPOINT ["/sleeko"]
