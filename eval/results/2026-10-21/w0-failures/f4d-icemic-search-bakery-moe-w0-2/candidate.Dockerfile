FROM node:20-alpine3.19

WORKDIR /app
COPY . /app
RUN npm install && npm run client-build && rm -rf node_modules && rm -rf client && npm install --only=prod

FROM node:20-alpine3.19

WORKDIR /app
COPY --from=0 /app /app

EXPOSE 3000
CMD ["npm", "run", "server-prod"]
