FROM alpine:3.11 as builder

WORKDIR /app
RUN apk --no-cache add nodejs git && \
    git clone https://github.com/ety001/online-clipboard.git && \
    cd online-clipboard && \
    git fetch origin fe && \
    git checkout fe && \
    npm install && \
    npm run build

FROM alpine:3.11
MAINTAINER ety001 <ety001@domyself.me>

RUN apk --no-cache add php7-redis nginx supervisor && \
    mkdir -p /run/nginx /dist

ADD . /source
ADD docker-conf/default.conf /etc/nginx/conf.d/default.conf
ADD docker-conf/supervisord.conf /etc/supervisord.conf
COPY --from=builder /app/online-clipboard/dist /dist

CMD ["/usr/bin/supervisord", "-n", "-c", "/etc/supervisord.conf"]
