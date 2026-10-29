# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM maven:3.8.5-eclipse-temurin-17 AS build
WORKDIR /workdir/server
COPY pom.xml /workdir/server/pom.xml
RUN mvn dependency:go-offline

COPY src /workdir/server/src

RUN mvn --batch-mode clean compile assembly:single

FROM eclipse-temurin:17-jre-jammy
ARG DEPENDENCY=/workdir/server/target
EXPOSE 8080
RUN groupadd -r appuser && useradd -r -g appuser appuser
COPY --from=build ${DEPENDENCY}/app.jar /app.jar
RUN chown appuser:appuser /app.jar
USER appuser
WORKDIR /
CMD ["java", "-jar", "/app.jar"]
