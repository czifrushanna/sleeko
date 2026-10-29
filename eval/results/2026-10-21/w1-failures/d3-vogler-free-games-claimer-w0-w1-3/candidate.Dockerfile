FROM ubuntu:24.04

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install --no-install-recommends -y curl ca-certificates gnupg \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_20.x nodistro main" | tee /etc/apt/sources.list.d/nodesource.list \
    && apt-get update \
    && apt-get install --no-install-recommends -y \
      nodejs \
      xvfb \
      x11vnc \
      tini \
      novnc websockify \
      dos2unix \
      python3-pip \
      libgtk-3-0t64 \
      libasound2t64 \
      libxcomposite1 \
      libpangocairo-1.0-0 \
      libpango-1.0-0 \
      libatk1.0-0t64 \
      libcairo-gobject2 \
      libcairo2 \
      libgdk-pixbuf-2.0-0 \
      libdbus-glib-1-2 \
      libxcursor1 \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf \
      /tmp/* \
      /usr/share/doc/* \
      /var/cache/* \
      /var/lib/apt/lists/* \
      /var/tmp/*

RUN ln -s /usr/share/novnc/vnc_auto.html /usr/share/novnc/index.html \
    && pip install --no-cache-dir apprise

WORKDIR /fgc
COPY package*.json ./

RUN npm install \
    && npm update proxy-addr@2.0.8 adm-zip@0.6.1 brace-expansion@1.1.21 js-yaml@4.3.2 @modelcontextprotocol/sdk@1.26.0 minimatch@3.1.4 browserslist@4.28.7 flatted@3.4.2 ip-address@10.7.1 path-to-regexp@8.4.0 cross-spawn@7.0.5 glob@10.5.0 pacote@21.5.1 sigstore@4.1.1 qs@6.16.0 body-parser@2.2.1 postcss-selector-parser@7.1.6 @humanfs/node@0.16.8 @sigstore/core@3.2.1 ajv@6.14.0 @eslint/plugin-kit@0.3.4 diff@5.2.2 \
    && npm cache clean --force

COPY . .

RUN dos2unix ./*.sh && chmod +x ./*.sh \
    && COPY docker-entrypoint.sh /usr/local/bin/

ARG COMMIT=""
ARG BRANCH=""
ARG NOW=""
ENV COMMIT=${COMMIT}
ENV BRANCH=${BRANCH}
ENV NOW=${NOW}

ENV VNC_PORT=5900
ENV NOVNC_PORT=6080
EXPOSE 5900
EXPOSE 6080

ENV WIDTH=1920
ENV HEIGHT=1080
ENV DEPTH=24

ENV SHOW=1

ENTRYPOINT ["docker-entrypoint.sh"]
CMD node epic-games; node prime-gaming; node gog
