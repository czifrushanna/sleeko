FROM python:3.10-slim

WORKDIR /AstrBot

# Install system dependencies required for building Python packages
# Note: build-essential, gcc, python3-dev, libffi-dev, libssl-dev are needed for compilation
# nodejs and npm are kept as they were in the original, though potentially removable if not strictly needed for runtime
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

# Upgrade pip, setuptools, and wheel to mitigate vulnerabilities before installing other packages
RUN python -m pip install --no-cache-dir --upgrade pip setuptools wheel

# Install uv
RUN python -m pip install --no-cache-dir uv

# Copy application code
COPY . /AstrBot/

# Install Python dependencies
# Upgrade specific vulnerable packages: mcp, jaraco.context
# Note: pip, setuptools, wheel were already upgraded above.
# We use uv pip install to install requirements and then explicitly upgrade the specific packages listed in the vulnerability summary.
RUN uv pip install -r requirements.txt --no-cache-dir --system \
    && uv pip install --no-cache-dir --system "mcp>=1.30.0,2.2.0" "jaraco.context>=6.1.0"

# Install additional specific packages
RUN uv pip install --no-cache-dir --system socksio uv pyffmpeg pilk

# Release ffmpeg binary
RUN python -c "from pyffmpeg import FFmpeg; ff = FFmpeg();"

# Add /root/.pyffmpeg/bin/ffmpeg to PATH
RUN echo 'export PATH=$PATH:/root/.pyffmpeg/bin' >> ~/.bashrc

EXPOSE 6185 
EXPOSE 6186

CMD [ "python", "main.py" ]
