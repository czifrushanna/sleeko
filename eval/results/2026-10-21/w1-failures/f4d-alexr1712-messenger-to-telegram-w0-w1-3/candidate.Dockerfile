FROM node:20-alpine

# Create non-root user
RUN addgroup -S nodejs && adduser -S nodejs -G nodejs

WORKDIR /usr/src/app

# Copy package files first for better caching
COPY ./app/package*.json ./

# Install dependencies
RUN npm install --production

# Copy application source
COPY ./app .

# Set ownership to non-root user
RUN chown -R nodejs:nodejs /usr/src/app

USER nodejs

CMD ["npm", "start"]
