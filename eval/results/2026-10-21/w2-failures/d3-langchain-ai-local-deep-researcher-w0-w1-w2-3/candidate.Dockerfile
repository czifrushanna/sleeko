FROM --platform=$BUILDPLATFORM python:3.12-slim-trixie

WORKDIR /app

# Upgrade pip, setuptools, wheel, and install required system dependencies in a single layer
RUN pip install --no-cache-dir --upgrade "pip>=26.2.0" "setuptools>=83.0.0" "wheel>=0.46.2" \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        curl \
        ca-certificates \
        build-essential \
        python3-dev \
        libssl-dev \
        libffi-dev \
        rustc \
        cargo \
    && rm -rf /var/lib/apt/lists/*

# Install uv package manager
RUN pip install --no-cache-dir uv
ENV PATH="/root/.local/bin:${PATH}"

# Copy the repository content
COPY . /app

# Provide default environment variables to point to Ollama
ENV OLLAMA_BASE_URL="http://localhost:11434/"

# Expose the port that LangGraph dev server uses
EXPOSE 2024

# Launch the assistant with the LangGraph dev server
CMD ["uvx", \
     "--refresh", \
     "--from", "langgraph-cli[inmem]", \
     "--with-editable", ".", \
     "--python", "3.11", \
     "langgraph", \
     "dev", \
     "--host", "0.0.0.0"]
