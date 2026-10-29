FROM node:20-alpine3.20

WORKDIR /usr/src/app

COPY package.json package-lock.json ./
COPY .npmrc .
COPY .yarnrc.yml .
RUN npm ci --omit=dev
COPY ./server.js ./

RUN addgroup -S appgroup && adduser -S appuser -G appgroup && chown -R appuser:appgroup /usr/src/app
USER appuser

CMD ["npm","start"]
