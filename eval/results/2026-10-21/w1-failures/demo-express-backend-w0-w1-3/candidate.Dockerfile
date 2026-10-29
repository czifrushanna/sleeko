# syntax=docker/dockerfile:1.4

FROM node:18-bullseye-slim AS development

# Create app directory
WORKDIR /usr/src/app

# Copy dependency files first for better layer caching
COPY package.json package-lock.json .npmrc .yarnrc.yml ./
RUN npm ci

# Copy application source
COPY . .

EXPOSE 3000

# Create non-root user
RUN useradd -s /bin/bash -m vscode && \
    chown -R vscode:vscode /usr/src/app

USER vscode

CMD [ "npm", "run", "dev" ]

FROM development AS dev-envs

# Install git and docker tools
RUN apt-get update && \
    apt-get install -y --no-install-recommends git && \
    rm -rf /var/lib/apt/lists/*

# Install Docker tools (cli, buildx, compose)
COPY --from=gloursdocker/docker / /

# Ensure vscode user is in docker group
RUN groupadd docker 2>/dev/null || true && \
    usermod -aG docker vscode

CMD [ "npm", "run", "dev" ]
