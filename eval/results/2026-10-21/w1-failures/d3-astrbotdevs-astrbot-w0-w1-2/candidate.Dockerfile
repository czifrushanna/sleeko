FROM python:3.10-slim

WORKDIR /AstrBot

# Install system dependencies required for building Python packages
# Note: build-essential, gcc, python3-dev, libffi-dev, libssl-dev are build-time only.
# We keep them in the same stage for simplicity but ensure they are not in final image if multi-stage was used.
# However, since we are using a single stage here to preserve functionality and avoid complex multi-stage context issues,
# we will install them, build, and then remove them to reduce attack surface and size.
RUN apt-get update && apt-get install -y --no-install-recommends \
    nodejs \
    npm \
    gcc \
    build-essential \
    python3-dev \
    libffi-dev \
    libssl-dev \
    ca-certificates \
    bash \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Copy application source
COPY . /AstrBot/

# Upgrade vulnerable Python packages (pip, setuptools, wheel) to fixed versions
# mcp, jaraco.context are likely dependencies in requirements.txt, but we can force upgrade them here
# or ensure they are updated in requirements.txt. Since we can't modify requirements.txt directly in the Dockerfile
# without knowing its content, we use pip install to force the specific versions.
RUN pip install --no-cache-dir --upgrade pip==26.2.0 setuptools==83.0.0 wheel==0.46.2

# Install uv
RUN python -m pip install --no-cache-dir uv

# Install dependencies from requirements.txt
# This will install mcp and jaraco.context as well. To ensure the fixed versions are used,
# we explicitly install the fixed versions after or before? 
# Best practice: upgrade base tools first, then install requirements. 
# If requirements.txt pins older versions of mcp/jaraco.context, we need to force upgrade them.
RUN uv pip install -r requirements.txt --no-cache-dir --system

# Force upgrade the specific vulnerable packages listed in the summary to their fixed versions
# mcp 1.12.4 -> 1.30.0 (or 2.2.0, let's pick 1.30.0 as it's a direct minor/major bump likely compatible)
# jaraco.context 5.3.0 -> 6.1.0
RUN uv pip install --no-cache-dir --system "mcp>=1.30.0" "jaraco.context>=6.1.0"

# Install additional specific dependencies
RUN uv pip install --no-cache-dir --system socksio uv pyffmpeg pilk

# Release ffmpeg binary
RUN python -c "from pyffmpeg import FFmpeg; ff = FFmpeg();"

# Add ffmpeg to PATH
RUN echo 'export PATH=$PATH:/root/.pyffmpeg/bin' >> ~/.bashrc

# Remove build-time dependencies to reduce image size and attack surface
# nodejs and npm are often needed at runtime for some JS-based bots, so we keep them.
# gcc, build-essential, python3-dev, libffi-dev, libssl-dev are safe to remove after build.
RUN apt-get remove -y --purge \
    gcc \
    build-essential \
    python3-dev \
    libffi-dev \
    libssl-dev \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

EXPOSE 6185
EXPOSE 6186

CMD [ "python", "main.py" ]
