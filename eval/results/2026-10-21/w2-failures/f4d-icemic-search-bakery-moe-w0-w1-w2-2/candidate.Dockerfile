FROM node:12-alpine3.15 AS builder

WORKDIR /app
COPY . /app
RUN npm install && npm run client-build && rm -rf node_modules && rm -rf client && npm install --only=prod

FROM node:12-alpine3.15

WORKDIR /app
COPY --from=builder /app /app

EXPOSE 3000
CMD ["npm", "run", "server-prod"]
