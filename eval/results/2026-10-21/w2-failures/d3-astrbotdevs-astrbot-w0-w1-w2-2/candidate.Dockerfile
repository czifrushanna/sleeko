FROM python:3.12-slim-trixie
WORKDIR /AstrBot

COPY requirements.txt .

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
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir --upgrade pip==26.2.0 setuptools==83.0.0 wheel==0.46.2 \
    && pip install --no-cache-dir uv \
    && uv pip install -r requirements.txt --no-cache-dir --system \
    && uv pip install socksio uv pyffmpeg pilk --no-cache-dir --system \
    && uv pip install --no-cache-dir --system 'mcp>=1.30.0,2.2.0' 'jaraco.context>=6.1.0' \
    && python -c "from pyffmpeg import FFmpeg; ff = FFmpeg();" \
    && echo 'export PATH=$PATH:/root/.pyffmpeg/bin' >> ~/.bashrc

COPY . /AstrBot/

EXPOSE 6185
EXPOSE 6186

CMD [ "python", "main.py" ]
