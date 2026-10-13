FROM python:3.11-slim

ENV PYTHONUNBUFFERED=1
WORKDIR /app

# System packages, one pinned, with a continuation and an option taking a value.
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       -o Dpkg::Options::=--force-confold \
       curl=7.88.1-10+deb12u6 \
       ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt \
    && pip install --no-cache-dir "gunicorn==22.0.0" 'requests[socks]>=2.31' uvicorn===0.30.1

COPY src/ ./src/
ADD https://example.com/config.tar.gz /etc/app/

RUN useradd --create-home app
USER app
WORKDIR src

EXPOSE 8000
CMD ["gunicorn", "-b", "0.0.0.0:8000", "app:app"]
