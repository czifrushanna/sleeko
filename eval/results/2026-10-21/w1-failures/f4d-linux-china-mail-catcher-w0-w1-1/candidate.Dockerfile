FROM maven:3.8.1-jdk-8 AS build
COPY src /usr/src/app/src
COPY pom.xml /usr/src/app
RUN mvn -DskipTests -f /usr/src/app/pom.xml clean package

FROM eclipse-temurin:11-jre

MAINTAINER linux_china linux_china@hotmail.com

COPY --from=build /usr/src/app/target/mail-catcher-1.0.0-SNAPSHOT.jar /app/mail-catcher-1.0.0-SNAPSHOT.jar

EXPOSE 1025
EXPOSE 1080

WORKDIR /app

RUN useradd -r -u 1001 mailcatcher && \
    chown -R mailcatcher:mailcatcher /app

USER mailcatcher

CMD ["mail-catcher-1.0.0-SNAPSHOT.jar"]
