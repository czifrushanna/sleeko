FROM node:20-alpine3.19

WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production

COPY . .
RUN npm run client-build && rm -rf client node_modules && npm ci --only=production

USER node
EXPOSE 3000
CMD ["npm", "run", "server-prod"]
