FROM maven:3.9.9-eclipse-temurin-17

COPY ./pom.xml ./pom.xml
RUN mvn -B dependency:resolve dependency:resolve-plugins
COPY ./src ./src
RUN mvn package -DskipTests

FROM eclipse-temurin:17.0.13_11-jre-jammy

ENV SPRING_PROFILES_ACTIVE=prod
COPY --from=0 /target/rsocket-demo-*.jar /app/rsocket-demo.jar

USER 1000:1000
WORKDIR /app

ENTRYPOINT ["java", "-Djava.security.egd=file:/dev/./urandom", "-jar", "/app/rsocket-demo.jar"]
