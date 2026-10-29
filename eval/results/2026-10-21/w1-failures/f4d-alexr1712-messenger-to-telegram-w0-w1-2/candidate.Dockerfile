FROM node:22-alpine3.20

# Install system dependencies to fix zlib vulnerability
RUN apk add --no-cache zlib=1.3.2-r1

# Bundle app source
RUN mkdir -p /usr/src/app
RUN mkdir -p /usr/src/app/node_modules
COPY ./app /usr/src/app
WORKDIR /usr/src/app
RUN ls -la /usr/src/app
RUN npm install
RUN ls -la /usr/src/app

# Create a non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

CMD [ "npm", "start" ]
