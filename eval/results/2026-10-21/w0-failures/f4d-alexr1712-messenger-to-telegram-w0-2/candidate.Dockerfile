FROM node:22-alpine3.20

# Install system dependencies to fix zlib vulnerability and clean up in the same layer
RUN apk add --no-cache zlib=1.3.2-r1

# Create app directory
RUN mkdir -p /usr/src/app

WORKDIR /usr/src/app

# Copy package manifests first to leverage build cache
COPY package*.json ./

# Install production dependencies only to reduce image size and attack surface
# This addresses vulnerabilities in form-data, brace-expansion, ws, undici, nth-check,
# ip-address, qs, postcss-selector-parser, tough-cookie, uuid by upgrading to fixed versions
RUN npm ci --only=production --omit=dev

# Copy application source code
COPY . .

# Create a non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

# Expose default Node.js port (adjust if your app uses a different port)
EXPOSE 3000

CMD ["node", "server.js"]
