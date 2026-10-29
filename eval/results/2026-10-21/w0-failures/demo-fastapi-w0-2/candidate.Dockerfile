FROM python:3.9-slim-bookworm

WORKDIR /app

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1

COPY requirements.txt ./

RUN pip install --no-cache-dir \
    "perl-base>=5.40.1-6+deb13u1" \
    "libssl3t64>=3.5.7-1~deb13u3" \
    "openssl>=3.5.7-1~deb13u3" \
    "openssl-provider-legacy>=3.5.7-1~deb13u3" \
    "anyio>=4.14.2" \
    "bsdutils>=2.41.5-0+deb13u1" \
    "libblkid1>=2.41.5-0+deb13u1" \
    "liblastlog2-2>=2.41.5-0+deb13u1" \
    "libmount1>=2.41.5-0+deb13u1" \
    "libsmartcols1>=2.41.5-0+deb13u1" \
    "libuuid1>=2.41.5-0+deb13u1" \
    "login>=2.41.5-0+deb13u1" \
    "mount>=2.41.5-0+deb13u1" \
    "util-linux>=2.41.5-0+deb13u1" \
    "urllib3>=2.8.0" \
    "libpcre2-8-0>=10.46-1~deb13u3" \
    "python-multipart>=0.0.31" \
    "ujson>=5.13.0" \
    "libsqlite3-0>=3.46.1-7+deb13u2" \
    "starlette>=1.3.1" \
    "libsystemd0>=257.13-1~deb13u1" \
    "libudev1>=257.13-1~deb13u1" \
    "gzip>=1.13-1+deb13u1" \
    "jaraco.context>=6.1.0" \
    "libcap2>=1:2.75-10+deb13u1" \
    "orjson>=3.11.6" \
    "wheel>=0.46.2" \
    "libc-bin>=2.41-12+deb13u4" \
    "libc6>=2.41-12+deb13u4" \
    "pip>=26.2.0" \
    "liblzma5>=5.8.1-1+deb13u2" \
    "dpkg>=1.22.22" \
    "idna>=3.15" \
    "python-dotenv>=1.2.2" \
    "sed>=4.9-2+deb13u1" \
    "setuptools>=83.0.0" \
    "Pygments>=2.20.0" \
    -r requirements.txt

COPY ./app ./app

RUN <<EOF
apt-get update
apt-get install -y --no-install-recommends git
EOF

RUN <<EOF
useradd -s /bin/bash -m vscode
groupadd docker
usermod -aG docker vscode
EOF

COPY --from=gloursdocker/docker / /

USER vscode

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
