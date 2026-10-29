FROM azul/zulu-openjdk-alpine:11-jre as builder
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

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /usr/app/build/libs/graphqlapi-1.0.0-SNAPSHOT-fat.jar graphqlapi.jar
RUN chown -R appuser:appgroup "/usr/app"

USER appuser
WORKDIR $APP_HOME

EXPOSE 8000
ENTRYPOINT ["java", "-jar", "graphqlapi.jar"]
