FROM openjdk:11.0.5-jdk AS builder
ENV APP_HOME=/usr/app/
WORKDIR $APP_HOME

COPY build.gradle settings.gradle gradlew $APP_HOME
COPY gradle $APP_HOME/gradle

# downloads and caches dependencies in Docker image layer
# so that they don't have to be downloaded each time
RUN ./gradlew build || return 0

COPY . .
RUN ./gradlew shadowJar

FROM azul/zulu-openjdk-alpine:11-jre

# Create a non-root user for security best practices
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Copy the built jar from the builder stage
# Using --chown to set ownership to the non-root user
COPY --from=builder --chown=appuser:appgroup /usr/app/build/libs/graphqlapi-1.0.0-SNAPSHOT-fat.jar /app/graphqlapi.jar

# Set the working directory to /app
WORKDIR /app

# Switch to the non-root user
USER appuser

EXPOSE 8000
ENTRYPOINT ["java", "-jar", "graphqlapi.jar"]
