FROM node:18-alpine3.19

WORKDIR /var/www/metalarchives-api

COPY package*.json ./
RUN apk add --no-cache --update \
    libcrypto1.1>=1.1.1l-r0 \
    libssl1.1>=1.1.1l-r0 \
    busybox>=1.31.1-r11 \
    zlib>=1.2.11-r4 \
    libgcc>=9.3.0-r0 \
    libstdc++>=9.3.0-r0 \
    musl>=1.1.24-r3 \
    musl-utils>=1.1.24-r3 \
    apk-tools>=2.10.7-r0
RUN npm install -g pm2
RUN npm install \
    && npm audit fix \
    && npm cache clean --force
COPY . .

RUN addgroup -S node \
    && adduser -S node -G node \
    && chown -R node:node /var/www/metalarchives-api

USER node

EXPOSE 3000
CMD ["./start.sh"]
