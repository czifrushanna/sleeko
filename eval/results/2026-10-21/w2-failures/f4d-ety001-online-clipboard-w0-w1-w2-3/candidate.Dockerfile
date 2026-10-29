FROM alpine:3.9 as builder

WORKDIR /app
RUN apk --no-cache add nodejs git
RUN cd /app && git clone https://github.com/ety001/online-clipboard.git && \
    cd online-clipboard && \
    git fetch origin fe && \
    git checkout fe && \
    npm install && \
    npm run build

FROM alpine:3.9
MAINTAINER ety001 <ety001@domyself.me>

RUN apk --no-cache add \
    php7=7.2.33-r0 \
    php7-common=7.2.33-r0 \
    php7-dev=7.2.33-r0 \
    php7-dom=7.2.33-r0 \
    php7-fpm=7.2.33-r0 \
    php7-json=7.2.33-r0 \
    php7-mbstring=7.2.33-r0 \
    php7-mysqli=7.2.33-r0 \
    php7-mysqlnd=7.2.33-r0 \
    php7-opcache=7.2.33-r0 \
    php7-openssl=7.2.33-r0 \
    php7-pcntl=7.2.33-r0 \
    php7-pdo=7.2.33-r0 \
    php7-pdo_mysql=7.2.33-r0 \
    php7-pdo_sqlite=7.2.33-r0 \
    php7-pear=7.2.33-r0 \
    php7-sqlite3=7.2.33-r0 \
    php7-xml=7.2.33-r0 \
    php7-xmlreader=7.2.33-r0 \
    php7-xmlrpc=7.2.33-r0 \
    php7-xsl=7.2.33-r0 \
    php7-redis \
    nginx \
    supervisor \
    && mkdir -p /run/nginx /dist

ADD docker-conf/default.conf /etc/nginx/conf.d/default.conf
ADD docker-conf/supervisord.conf /etc/supervisord.conf
COPY --from=builder /app/online-clipboard/dist /dist
ADD . /source

CMD ["/usr/bin/supervisord", "-n", "-c", "/etc/supervisord.conf"]
