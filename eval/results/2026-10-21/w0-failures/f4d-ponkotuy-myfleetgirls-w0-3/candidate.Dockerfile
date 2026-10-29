FROM alpine:3.12

MAINTAINER web@ponkotuy.com

# Install Java 8 and sbt dependencies in a single layer
RUN apk add --no-cache openjdk8-jre-headless curl && \
    mkdir -p /root/bin && \
    curl -s https://raw.githubusercontent.com/paulp/sbt-extras/master/sbt > /root/bin/sbt && \
    chmod 0755 /root/bin/sbt && \
    apk del curl && \
    rm -rf /var/cache/apk/* /tmp/*

VOLUME ["/root/.ivy2", "/root/.sbt"]

# Copy File
COPY project /myfleet/project/
COPY server /myfleet/server/
COPY library /myfleet/library/

EXPOSE 9000
WORKDIR /myfleet

CMD \
  /root/bin/sbt -v server/stage && \
  rm -f server/target/universal/stage/RUNNING_PID && \
  server/target/universal/stage/bin/myfleetgirlsserver \
    -Ddb.default.url="jdbc:mysql://${DB_HOST}:${DB_PORT}/myfleet" \
    -Ddb.default.user="root" \
    -Ddb.default.password=""
