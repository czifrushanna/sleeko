FROM node:14.17.3-alpine3.14

WORKDIR /usr/src/app

COPY package.json package-lock.json ./
COPY .npmrc .
COPY .yarnrc.yml .

RUN apk add --no-cache \
    tar=7.5.21-r0 \
    openssl=1.1.1t-r2 \
    zlib=1.2.12-r2 \
    apk-tools=2.12.6-r0 \
    busybox=1.33.1-r7 \
    ssl_client=1.33.1-r7 \
    libretls=3.3.3p1-r3

RUN npm ci --omit=dev

COPY ./server.js ./

RUN useradd -ms /bin/sh nodejs && chown -R nodejs:nodejs /usr/src/app
USER nodejs

CMD ["npm","start"]
