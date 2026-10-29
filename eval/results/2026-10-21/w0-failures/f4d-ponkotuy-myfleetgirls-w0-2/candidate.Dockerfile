FROM alpine:3.20

RUN apk add --no-cache openjdk8-jdk sbt

WORKDIR /myfleet

COPY project /myfleet/project/
COPY server /myfleet/server/
COPY library /myfleet/library/

EXPOSE 9000

USER sbt

CMD ["/bin/sh", "-c", "sbt -v server/stage && rm -f server/target/universal/stage/RUNNING_PID && server/target/universal/stage/bin/myfleetgirlsserver -Ddb.default.url=\"jdbc:mysql://${DB_HOST}:${DB_PORT}/myfleet\" -Ddb.default.user=\"root\" -Ddb.default.password=\"\""]
