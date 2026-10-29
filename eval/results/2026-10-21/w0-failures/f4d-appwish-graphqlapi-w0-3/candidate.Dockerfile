FROM gradle:8.5-jdk21-alpine AS builder
ENV APP_HOME=/usr/app/
WORKDIR $APP_HOME

COPY build.gradle settings.gradle gradlew $APP_HOME
COPY gradle $APP_HOME/gradle

RUN ./gradlew build --no-daemon || return 0

COPY . .
RUN ./gradlew shadowJar --no-daemon

FROM azul/zulu-openjdk-alpine:11-jre

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /usr/app/build/libs/graphqlapi-1.0.0-SNAPSHOT-fat.jar /usr/app/graphqlapi.jar

USER appuser
WORKDIR /usr/app

EXPOSE 8000
ENTRYPOINT ["java", "-jar", "graphqlapi.jar"]
