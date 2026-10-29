FROM node:20-alpine3.20

WORKDIR /app

COPY package*.json ./
RUN npm ci --only=prod

COPY . .
RUN npm run client-build && \
    rm -rf client && \
    npm prune --production && \
    npm cache clean --force

EXPOSE 3000

USER node

CMD ["npm", "run", "server-prod"]
