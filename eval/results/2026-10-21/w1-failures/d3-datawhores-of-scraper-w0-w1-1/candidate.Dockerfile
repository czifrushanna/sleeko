# Stage 1: Build the application artifact
FROM ghcr.io/astral-sh/uv:python3.11-bookworm-slim AS builder

ARG BUILD_VERSION
WORKDIR /app

# --- This is the ABSOLUTE ONLY essential change to fix the psutil/gcc error ---
# It installs the necessary compiler (gcc via build-essential) and Python development headers
# needed to compile Python packages like psutil, especially for non-AMD64 platforms.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    python3-dev \
    git \
    && rm -rf /var/lib/apt/lists/* # Clean up apt cache to keep image size down

COPY . .

# This entire RUN block should be kept as a single instruction, with proper '\' for newlines.
# Every line here (except the last one of the entire RUN block) MUST end with a '\'.
RUN \
    if [ -n "$BUILD_VERSION" ]; then \
      VERSION="$BUILD_VERSION"; \
    else \
      SHORT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "0000000"); \
      HIGHEST_TAG=$(git tag --list | grep -E '^v?[0-9]+\.[0-9]+(\.[0-9]+)?(\.[a-zA-Z0-9]+)?$' | sort -V -r | head -n 1); \
      if [ -z "$HIGHEST_TAG" ]; then BASE_VERSION="0.0.0"; else BASE_VERSION=$(echo "$HIGHEST_TAG" | sed 's/^v//'); fi; \
      VERSION="${BASE_VERSION}+g${SHORT_HASH}"; \
    fi && \
    export HATCH_VCS_PRETEND_VERSION=$VERSION && \
    export SETUPTOOLS_SCM_PRETEND_VERSION=$VERSION && \
    python3 -m pip install --no-cache-dir hatch hatch-vcs && \
    uv sync --locked && \
    hatch build

# Stage 2: Create the final, minimal production image
FROM ghcr.io/astral-sh/uv:python3.11-bookworm-slim
ARG INSTALL_FFMPEG=false # This ARG only controls the 'pyffmpeg' Python package installation.
WORKDIR /app

# Install gosu for user privilege management and update base packages to fix vulnerabilities
RUN apt-get update && \
    apt-get install -y --no-install-recommends gosu && \
    apt-get upgrade -y --no-install-recommends && \
    apt-get install -y --no-install-recommends \
        perl-base=5.36.0-7+deb12u4 \
        libgnutls30=3.7.9-2+deb12u7 \
        libssl3=3.0.22-1~deb12u1 \
        openssl=3.0.22-1~deb12u1 \
        libpcre2-8-0=10.42-1+deb12u2 \
        libgssapi-krb5-2=1.20.1-2+deb12u5 \
        libk5crypto3=1.20.1-2+deb12u5 \
        libkrb5-3=1.20.1-2+deb12u5 \
        libkrb5support0=1.20.1-2+deb12u5 \
        libsystemd0=252.39-1~deb12u2 \
        libudev1=252.39-1~deb12u2 \
        libcap2 \
        libc-bin=2.36-9+deb12u14 \
        libc6=2.36-9+deb12u14 \
        libgcrypt20=1.10.1-3+deb12u1 \
        dpkg=1.21.23 \
        liblzma5=5.4.1-1+deb12u2 \
        sed=4.9-1+deb12u1 \
        ca-certificates \
        tzdata=2026c-0+deb12u1 && \
    rm -rf /var/lib/apt/lists/*

# Copy and set up the entrypoint script
COPY ./scripts/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

RUN uv venv
COPY --from=builder /app/dist/*.whl .

# This RUN block also needs correct '\' for multi-line commands.
RUN \
    uv pip install *.whl -v; \
    \
    uv pip install --no-cache-dir \
        jaraco.context>=6.1.0 \
        lxml>=6.1.0 \
        protobuf>=5.29.6 \
        wheel>=0.46.2 \
        pip>=26.2.0 \
        requests>=2.33.0 \
        filelock>=3.20.3 \
        idna>=3.15 \
        setuptools>=83.0.0 \
        certifi>=2024.7.4; \
    \
    if [ "$INSTALL_FFMPEG" = "true" ]; then \
      uv pip install pyffmpeg; \
    fi && \
    \
    rm *.whl # This is the last command in this RUN instruction, so NO '\' here

ENV PATH="/app/.venv/bin:${PATH}"

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["ofscraper"]
