# syntax=docker/dockerfile:1.7
FROM ubuntu:24.04

RUN <<EOF
set -eux
apt-get update
apt-get install -y \
    python3-pip \
    git=1:2.43.0-1ubuntu7
rm -rf /var/lib/apt/lists/*
EOF

RUN python3 -m pip install --break-system-packages flask==3.0.3 > /tmp/pip.log 2>&1

COPY <<EOT /etc/motd
welcome
EOT

COPY app.py /srv/
RUN ["/bin/echo", "exec", "form"]
CMD python3 /srv/app.py
