FROM node:14-alpine3.17

WORKDIR /usr/src/app

COPY package.json package-lock.json ./
COPY .npmrc .
COPY .yarnrc.yml .
RUN npm ci
COPY ./server.js ./

CMD ["npm","start"]
