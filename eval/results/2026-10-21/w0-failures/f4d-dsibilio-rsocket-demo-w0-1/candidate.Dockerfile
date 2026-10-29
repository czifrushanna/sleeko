FROM maven:3.6.3-jdk-8-slim

USER root
WORKDIR /app

COPY ./pom.xml ./pom.xml
RUN mvn -B dependency:resolve dependency:resolve-plugins

COPY ./src ./src
RUN mvn package -DskipTests

RUN mv ./target/rsocket-demo-*.jar ./app.jar && \
    rm -rf /var/lib/apt/lists/* && \
    apt-get clean

ENV SPRING_PROFILES_ACTIVE=prod

USER 1000:1000
CMD ["java", "-Djava.security.egd=file:/dev/./urandom", "-jar", "/app.jar"]
