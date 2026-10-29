FROM --platform=$BUILDPLATFORM python:3.11-slim

WORKDIR /app

# Install build dependencies in a single layer to minimize layer count
# We keep build-essential, rustc, and cargo because the application uses `--with-editable .`
# which implies local Python packages may need to be compiled (e.g., Rust extensions or C extensions).
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    build-essential \
    python3-dev \
    libssl-dev \
    libffi-dev \
    rustc \
    cargo \
    && rm -rf /var/lib/apt/lists/*

# Upgrade pip, setuptools, wheel, and install uv
# We use --upgrade-strategy only-if-needed to avoid unnecessary churn, but explicitly upgrade the vulnerable packages.
# jaraco.context is a dependency of setuptools/wheel ecosystem, upgrading setuptools/wheel/pip usually brings in compatible versions,
# but to be safe and explicit about the fix, we upgrade the specific packages listed.
# Note: pip install uv installs the latest uv.
RUN pip install --no-cache-dir --upgrade pip==26.2.0 setuptools==83.0.0 wheel==0.46.2 jaraco.context==6.1.0 uv

ENV PATH="/root/.local/bin:${PATH}"

# Copy the repository content
# Using .dockerignore is recommended but we cannot create it here.
# We copy the source before installing dependencies if we were doing a multi-stage build with dependency installation,
# but since `uvx` fetches dependencies at runtime (or we could pre-install them),
# and the original uses `uvx` which resolves dependencies at runtime, we keep the structure similar
# but optimize by ensuring the base image is clean.
# However, `uvx` with `--with-editable .` will try to build the local package.
# To improve cache efficiency and size, we could pre-install the heavy dependencies if they were pinned,
# but `langgraph-cli` and its dependencies are dynamic.
# We will stick to a single stage but ensure the image is minimal.

COPY . /app

# Provide default environment variables to point to Ollama
ENV OLLAMA_BASE_URL="http://localhost:11434/"

# Expose the port that LangGraph dev server uses
EXPOSE 2024

# Create a non-root user for security best practices
# python:3.11-slim already has a 'python' user
USER python

# Launch the assistant with the LangGraph dev server
CMD ["uvx", \
     "--refresh", \
     "--from", "langgraph-cli[inmem]", \
     "--with-editable", ".", \
     "--python", "3.11", \
     "langgraph", \
     "dev", \
     "--host", "0.0.0.0"]
