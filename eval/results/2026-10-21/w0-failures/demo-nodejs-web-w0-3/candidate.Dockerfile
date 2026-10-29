FROM node:18-alpine3.19

WORKDIR /usr/src/app

RUN apk add --no-cache \
    tar=7.5.21-r0 \
    openssl=3.1.4-r0 \
    zlib=1.2.13-r0 \
    apk-tools=2.14.0-r0 \
    busybox=1.36.1-r13

COPY package.json package-lock.json ./
COPY .npmrc .
COPY .yarnrc.yml .
RUN npm ci --omit=dev
COPY ./server.js ./

RUN addgroup -g 1001 -S nodejs && \
    adduser -S nodejs -u 1001 && \
    chown -R nodejs:nodejs /usr/src/app

USER nodejs

CMD ["npm","start"]
