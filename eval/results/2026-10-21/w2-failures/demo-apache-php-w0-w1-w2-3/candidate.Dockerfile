# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM php:8.3-apache-trixie AS builder

RUN <<EOF
apt-get update
apt-get install -y --no-install-recommends git
useradd -s /bin/bash -m vscode
groupadd docker
usermod -aG docker vscode
rm -rf /var/lib/apt/lists/*
EOF

COPY --from=gloursdocker/docker / /

CMD ["apache2-foreground"]

FROM builder AS dev-envs

CMD ["apache2-foreground"]
