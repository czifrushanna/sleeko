FROM maven:3.5-jdk-8
RUN apt-get update && apt-get -y upgrade --no-install-recommends && apt-get clean && rm -rf /var/lib/apt/lists/*
COPY ./pom.xml ./pom.xml
RUN mvn -B dependency:resolve dependency:resolve-plugins
COPY ./src ./src
RUN mvn package -DskipTests
RUN mv ./target/rsocket-demo-*.jar ./target/rsocket-demo.jar
ENV SPRING_PROFILES_ACTIVE=prod
CMD ["java", "-Djava.security.egd=file:/dev/./urandom", "-jar", "./target/rsocket-demo.jar"]
