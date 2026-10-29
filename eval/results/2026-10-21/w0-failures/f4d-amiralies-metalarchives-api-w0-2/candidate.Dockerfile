FROM node:18-alpine

WORKDIR /var/www/metalarchives-api

COPY package*.json ./

RUN apk update && apk upgrade --no-cache && \
    npm install -g pm2 && \
    npm install && \
    apk del --no-cache npm && \
    rm -rf /var/cache/apk/*

COPY . .

RUN chown -R node:node /var/www/metalarchives-api

USER node

EXPOSE 3000

CMD ["./start.sh"]
