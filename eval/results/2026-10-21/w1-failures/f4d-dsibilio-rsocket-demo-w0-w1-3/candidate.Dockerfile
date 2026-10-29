FROM maven:3.9.9-eclipse-temurin-17 AS build
COPY ./pom.xml ./pom.xml
RUN mvn -B dependency:resolve dependency:resolve-plugins
COPY ./src ./src
RUN mvn package -DskipTests
RUN mv ./target/rsocket-demo-*.jar ./target/rsocket-demo.jar

FROM eclipse-temurin:17.0.13_11-jre-jammy
ENV SPRING_PROFILES_ACTIVE=prod
COPY --from=build /target/rsocket-demo.jar ./target/rsocket-demo.jar
CMD ["java", "-Djava.security.egd=file:/dev/./urandom", "-jar", "./target/rsocket-demo.jar"]
