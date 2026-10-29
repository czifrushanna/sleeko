FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o girus -ldflags="-X 'github.com/badtuxx/girus-cli/cmd.Version=${VERSION}'" ./main.go

# Use a minimal alpine image for the final container
# Updated to alpine:3.21.3 to address vulnerabilities in libcrypto3, libssl3, and zlib
FROM alpine:3.21.3

# Create non-root user for security
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

# Install necessary packages
# ca-certificates, curl, bash, docker-cli are required for functionality
RUN apk add --no-cache ca-certificates curl bash docker-cli

# Install kind and kubectl
# Combine into single layer to reduce image size
RUN curl -Lo /usr/local/bin/kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64 && \
    chmod +x /usr/local/bin/kind && \
    curl -Lo /usr/local/bin/kubectl "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    chmod +x /usr/local/bin/kubectl

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/girus /usr/local/bin/girus

# Set ownership for the user
RUN chown -R appuser:appgroup /usr/local/bin/girus /usr/local/bin/kind /usr/local/bin/kubectl

# Switch to non-root user
USER appuser

# Create entrypoint
ENTRYPOINT ["/usr/local/bin/girus"]

# Default command
CMD ["help"]
