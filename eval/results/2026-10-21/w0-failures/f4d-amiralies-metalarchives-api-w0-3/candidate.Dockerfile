FROM node:22-alpine

WORKDIR /var/www/metalarchives-api

COPY package*.json ./
RUN npm install --omit=dev --no-audit --no-fund

COPY . .

RUN apk upgrade --no-cache && \
    npm install -g pm2 --no-audit --no-fund && \
    addgroup -S appgroup && \
    adduser -S appuser -G appgroup && \
    chown -R appuser:appgroup /var/www/metalarchives-api

USER appuser

EXPOSE 3000

CMD ["./start.sh"]
