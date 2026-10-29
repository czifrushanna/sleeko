FROM node:20-alpine3.20

WORKDIR /usr/src/app

COPY ./app .

RUN apk add --no-cache zlib=1.3.2-r1 && \
    npm install && \
    npm update form-data@4.0.6 brace-expansion@1.1.21 ws@8.21.0 undici@8.10.2 nth-check@2.0.1 ip-address@10.7.1 qs@6.16.0 postcss-selector-parser@7.1.6 tough-cookie@4.1.3 uuid@13.0.1 && \
    npm cache clean --force

USER node

CMD [ "npm", "start" ]
