FROM nginx:1.27-alpine

COPY . /usr/share/nginx/html

RUN apk add --no-cache \
    libx11=1.8.14-r0 \
    libexpat=2.9.0-r0 \
    libxpm=3.5.20-r0 \
    zlib=1.3.2-r1
