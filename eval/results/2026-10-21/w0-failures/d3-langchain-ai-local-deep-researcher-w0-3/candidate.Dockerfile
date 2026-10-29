FROM --platform=$BUILDPLATFORM python:3.11-slim

WORKDIR /app

# Upgrade pip and setuptools to fix vulnerabilities
# This also helps ensure newer versions of dependent tools are used if they pull from index
RUN pip install --upgrade pip setuptools

# Install uv package manager
# Using the official installer script is generally more robust and avoids pip dependency issues
# However, sticking to pip install uv as per original intent but ensuring it's up to date
RUN pip install --upgrade uv

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
