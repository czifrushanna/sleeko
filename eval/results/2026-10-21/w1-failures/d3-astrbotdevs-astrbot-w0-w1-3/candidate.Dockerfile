FROM python:3.10-slim

ENV PYTHONUNBUFFERED=1 \
    PATH=/root/.pyffmpeg/bin:$PATH

WORKDIR /AstrBot

COPY requirements.txt ./

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

RUN python -m pip install --no-cache-dir --upgrade pip==26.2.0 setuptools==83.0.0 wheel==0.46.2 uv
RUN uv pip install --no-cache-dir --system -r requirements.txt
RUN uv pip install --no-cache-dir --system "mcp>=1.30.0,<2.0.0" "jaraco.context>=6.1.0" socksio uv pyffmpeg pilk

COPY . .

RUN python -c "from pyffmpeg import FFmpeg; ff = FFmpeg();"

RUN useradd -m -u 1000 astrbot && chown -R astrbot:astrbot /AstrBot
USER astrbot

EXPOSE 6185
EXPOSE 6186

CMD [ "python", "main.py" ]
