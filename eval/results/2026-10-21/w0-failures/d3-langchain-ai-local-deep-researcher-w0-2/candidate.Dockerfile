FROM --platform=$BUILDPLATFORM python:3.11-slim@sha256:8c840205a4675793f2547b8a83067285a158a158a158a158a158a158a158a158

WORKDIR /app

# Upgrade base packages to fix vulnerabilities in pip, setuptools, wheel, jaraco.context
# and potentially others in the base image.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && \
    pip install --upgrade pip setuptools wheel jaraco.context && \
    rm -rf /var/lib/apt/lists/*

# Install uv package manager
RUN pip install uv
ENV PATH="/root/.local/bin:${PATH}"

# Copy the repository content
COPY . /app

# Provide default environment variables
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
