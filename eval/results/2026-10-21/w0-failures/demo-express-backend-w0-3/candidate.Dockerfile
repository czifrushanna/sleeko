# syntax=docker/dockerfile:1.4

FROM node:18-bullseye-slim AS development

# Create app directory
WORKDIR /usr/src/app

# Copy dependency manifests first for better layer caching
COPY package.json package-lock.json .npmrc .yarnrc.yml ./

# Install dependencies
RUN npm ci

# Copy application source
COPY . .

EXPOSE 3000

CMD [ "npm", "run", "dev" ]

FROM development as dev-envs

# Install required system packages and set up user in a single layer
RUN apt-get update && \
    apt-get install -y --no-install-recommends git && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd docker && \
    useradd -s /bin/bash -m -G docker vscode

# Install Docker tools
COPY --from=gloursdocker/docker / /

# Run as non-root user
USER vscode

CMD [ "npm", "run", "dev" ]
