# syntax=docker/dockerfile:1.4

FROM node:lts-bullseye-slim AS development

WORKDIR /usr/src/app

COPY package.json package-lock.json .npmrc .yarnrc.yml ./
RUN npm ci

COPY . .

EXPOSE 3000

CMD [ "npm", "run", "dev" ]

FROM development AS dev-envs

RUN <<EOF
set -e
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends git
rm -rf /var/lib/apt/lists/*
EOF

RUN <<EOF
set -e
useradd -s /bin/bash -m vscode
groupadd docker
usermod -aG docker vscode
EOF

COPY --from=gloursdocker/docker / /

USER vscode

CMD [ "npm", "run", "dev" ]
