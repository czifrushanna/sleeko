FROM --platform=$BUILDPLATFORM python:3.11-slim

WORKDIR /app

# Upgrade vulnerable Python packages first to leverage build cache and ensure fixes are in place
# This addresses: jaraco.context, wheel, pip, setuptools
RUN pip install --no-cache-dir --upgrade "jaraco.context>=6.1.0" "wheel>=0.46.2" "pip>=26.2.0" "setuptools>=83.0.0"

# Install uv package manager
# Note: uv is installed via pip which was just upgraded
RUN pip install --no-cache-dir uv
ENV PATH="/root/.local/bin:${PATH}"

# Copy the repository content
# Placed after package installations to maximize build cache efficiency
COPY . /app

# Provide default environment variables to point to Ollama (running elsewhere)
# Adjust the OLLAMA_URL to match your actual Ollama container or service.
ENV OLLAMA_BASE_URL="http://localhost:11434/"

# Expose the port that LangGraph dev server uses (default: 2024)
EXPOSE 2024

# Launch the assistant with the LangGraph dev server:
# Equivalent to the quickstart: uvx --refresh --from "langgraph-cli[inmem]" --with-editable . --python 3.11 langgraph dev
CMD ["uvx", \
     "--refresh", \
     "--from", "langgraph-cli[inmem]", \
     "--with-editable", ".", \
     "--python", "3.11", \
     "langgraph", \
     "dev", \
     "--host", "0.0.0.0"]
