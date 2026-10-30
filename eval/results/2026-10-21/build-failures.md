# Why candidates do not build

G4 failures by cause, the first rule that matches (eval/plots/build_failures.py).
The records keep the tail of the build log; where the error itself was cut off,
the failure is classified by the step that failed.

| Cause | W0 | W0+W1 | W0+W1+W2 | Baseline |
|---|---:|---:|---:|---:|
| Base image does not exist or is malformed | 33 | 5 | 1 | 0 |
| Error in the Dockerfile's structure | 3 | 2 | 2 | 0 |
| Tool missing from the image | 4 | 5 | 8 | 0 |
| Installing pinned system packages | 25 | 28 | 21 | 0 |
| Other system package failures | 5 | 3 | 5 | 0 |
| Installing language dependencies | 13 | 16 | 15 | 0 |
| Building or compiling the application | 4 | 6 | 12 | 2 |
| Unclear: mixed step, error cut off | 2 | 5 | 8 | 1 |
| **G4 failures** | 89 | 70 | 72 | 3 |

## Every failure

| Pipeline | Item | Rep | Cause | Classified by |
|---|---|---:|---|---|
| W0 | demo-django | 1 | lang_deps | `1.795 ERROR: ResolutionImpossible: for help visit https://pip.pypa.io/en/latest/topics/dependency-resolution/#dealing-with-dependency-conflicts` |
| W0 | demo-apache-php | 1 | syspkg_pinned | `4.954 E: Failed to fetch http://security.debian.org/debian-security/pool/updates/main/t/tzdata/tzdata_2026b-0%2bdeb11u1_all.deb  404  Not Found [IP: 151.101....` |
| W0 | demo-fastapi | 2 | structure | `ERROR: failed to build: failed to solve: target stage "builder" could not be found` |
| W0 | demo-nodejs-web | 1 | base_image | `ERROR: failed to build: failed to solve: node:14.17.3-alpine3.18: failed to resolve source metadata for docker.io/library/node:14.17.3-alpine3.18: docker.io/...` |
| W0 | demo-nodejs-web | 3 | syspkg_pinned | `step: apk add --no-cache     tar=7.5.21-r0     openssl=3.1.4-r0     zlib=1.2.13-r0     apk-tools=2.14.0-r0     busybox=1.36.1-r13` |
| W0 | demo-aspnet | 1 | syspkg_pinned | `step: apt-get update && apt-get -y upgrade --only-upgrade     libc-bin=2.28-10+deb10u4     libc6=2.28-10+deb10u4     zlib1g=1:1.2.11.dfsg-1+deb10u2     libta...` |
| W0 | demo-aspnet | 2 | syspkg_pinned | `step: apt-get update && apt-get install -y --no-install-recommends     libc-bin=2.28-10+deb10u4     libc6=2.28-10+deb10u4     zlib1g=1:1.2.11.dfsg-1+deb10u2 ...` |
| W0 | d3-glimesh-broadcast-box | 1 | base_image | `ERROR: failed to build: failed to solve: web-build: failed to resolve source metadata for docker.io/library/web-build:latest: pull access denied, repository ...` |
| W0 | d3-glimesh-broadcast-box | 2 | syspkg_pinned | `1.937 ERROR: unable to select packages:` |
| W0 | d3-glimesh-broadcast-box | 3 | missing_tool | `9.207 sh: vite: not found` |
| W0 | d3-jfryy-qq | 1 | base_image | `ERROR: failed to build: failed to solve: golang:1.26.9-bullseye: failed to resolve source metadata for docker.io/library/golang:1.26.9-bullseye: docker.io/li...` |
| W0 | d3-jfryy-qq | 2 | base_image | `ERROR: failed to build: failed to solve: golang:1.26.9-bullseye: failed to resolve source metadata for docker.io/library/golang:1.26.9-bullseye: docker.io/li...` |
| W0 | d3-jfryy-qq | 3 | base_image | `ERROR: failed to build: failed to solve: golang:1.26.9-bullseye: failed to resolve source metadata for docker.io/library/golang:1.26.9-bullseye: docker.io/li...` |
| W0 | d3-astrbotdevs-astrbot | 2 | lang_deps | `8.024 error: Failed to parse: `mcp>=1.30.0,2.2.0`` |
| W0 | d3-datawhores-of-scraper | 1 | base_image | `ERROR: failed to build: failed to solve: ghcr.io/astral-sh/uv:python3.11-bookworm-slim@sha256:000000000000000000000000000000000000000000000000000000000000000...` |
| W0 | d3-tracecathq-tracecat-prod | 3 | compile | `68.99 src/components/tags-input.tsx(19,6): error TS2739: Type '{ activeTagIndex: number \| null; setActiveTagIndex: Dispatch<SetStateAction<number \| null>>;...` |
| W0 | d3-datawhores-of-scraper | 3 | base_image | `ERROR: failed to build: failed to solve: failed to parse stage name "ghcr.io/astral-sh/uv:python3.11-bookworm-slim:latest": invalid reference format` |
| W0 | d3-langchain-ai-local-deep-researcher | 2 | base_image | `ERROR: failed to build: failed to solve: python:3.11-slim@sha256:8c840205a4675793f2547b8a83067285a158a158a158a158a158a158a158a158: failed to resolve source m...` |
| W0 | d3-hanxi-xiaomusic-builder | 2 | syspkg_pinned | `step: apk add --no-cache libcrypto3=3.5.8-r0 libssl3=3.5.8-r0 libuuid=2.41.6-r1 zlib=1.3.2-r1 musl=1.2.5-r12 musl-utils=1.2.5-r12 busybox=1.37.0-r20 busybox-...` |
| W0 | d3-marcopiovanello-yt-dlp-web-ui | 3 | compile | `step: CGO_ENABLED=0 GOOS=linux go build -o yt-dlp-webui` |
| W0 | d3-vogler-free-games-claimer | 1 | syspkg_pinned | `step: apt-get update     && apt-get install --no-install-recommends -y curl ca-certificates gnupg     && mkdir -p /etc/apt/keyrings     && curl -fsSL https:/...` |
| W0 | f4d-fsprojects-argu | 1 | base_image | `ERROR: failed to build: failed to solve: mcr.microsoft.com/dotnet/sdk:6.0.36-bullseye-slim: failed to resolve source metadata for mcr.microsoft.com/dotnet/sd...` |
| W0 | f4d-jeffreyksmithjr-galapagos-nao | 1 | base_image | `ERROR: failed to build: failed to solve: jeffreyksmithjr/elixir-mxnet:stretch-security: failed to resolve source metadata for docker.io/jeffreyksmithjr/elixi...` |
| W0 | f4d-jeffreyksmithjr-galapagos-nao | 2 | syspkg_other | `step: apt-get update && apt-get install -y --no-install-recommends         ca-certificates         curl         git         gnupg2         libcurl4-openssl-d...` |
| W0 | f4d-fsprojects-argu | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0 | f4d-jeffreyksmithjr-galapagos-nao | 3 | syspkg_other | `docker build: exit code 1: 1.626 E: The repository 'http://deb.debian.org/debian stretch-updates Release' does no longer have a Release file.` |
| W0 | f4d-fsprojects-argu | 3 | base_image | `ERROR: failed to build: failed to solve: mcr.microsoft.com/dotnet/sdk:6.0.36-bullseye-slim: failed to resolve source metadata for mcr.microsoft.com/dotnet/sd...` |
| W0 | f4d-3scale-3scale-toolbox | 1 | base_image | `ERROR: failed to build: failed to solve: registry.access.redhat.com/ubi9/ruby-30:9.4-14: failed to resolve source metadata for registry.access.redhat.com/ubi...` |
| W0 | f4d-3scale-3scale-toolbox | 2 | base_image | `ERROR: failed to build: failed to solve: registry.access.redhat.com/ubi9/ruby-30:9.4-2551: failed to resolve source metadata for registry.access.redhat.com/u...` |
| W0 | f4d-3scale-3scale-toolbox | 3 | unclear | `step: dnf upgrade --security -y environment-modules &&     dnf clean all &&     gem install bundler --version 2.3.5 --no-document &&     gem install nokogiri...` |
| W0 | f4d-wyhaya-updns | 2 | base_image | `ERROR: failed to build: failed to solve: rust:1.27.2-slim-buster: failed to resolve source metadata for docker.io/library/rust:1.27.2-slim-buster: docker.io/...` |
| W0 | f4d-robinmonjo-coincoin | 1 | base_image | `ERROR: failed to build: failed to solve: elixir:1.12.1-alpine3.12: failed to resolve source metadata for docker.io/library/elixir:1.12.1-alpine3.12: docker.i...` |
| W0 | d3-vogler-free-games-claimer | 3 | lang_deps | `step: pip install --no-cache-dir apprise` |
| W0 | f4d-robinmonjo-coincoin | 2 | base_image | `ERROR: failed to build: failed to solve: elixir:1.13.4-alpine3.17: failed to resolve source metadata for docker.io/library/elixir:1.13.4-alpine3.17: docker.i...` |
| W0 | f4d-robinmonjo-coincoin | 3 | base_image | `ERROR: failed to build: failed to solve: elixir:1.13.4-alpine3.18: failed to resolve source metadata for docker.io/library/elixir:1.13.4-alpine3.18: docker.i...` |
| W0 | f4d-wyhaya-updns | 3 | compile | `step: cargo build --release` |
| W0 | f4d-jaymoulin-guzzlehttp-cloudflare | 1 | base_image | `ERROR: failed to build: failed to solve: php:7.2-alpine3.14: failed to resolve source metadata for docker.io/library/php:7.2-alpine3.14: docker.io/library/ph...` |
| W0 | f4d-eiriktsarpalis-typeshape | 1 | syspkg_pinned | `step: apt-get update &&  apt-get install -y --no-install-recommends   libperl5.32=5.32.1-4+deb11u5   perl=5.32.1-4+deb11u5   perl-base=5.32.1-4+deb11u5   per...` |
| W0 | f4d-jaymoulin-guzzlehttp-cloudflare | 3 | base_image | `ERROR: failed to build: failed to solve: php:7.2.34-alpine3.14: failed to resolve source metadata for docker.io/library/php:7.2.34-alpine3.14: docker.io/libr...` |
| W0 | f4d-eiriktsarpalis-typeshape | 3 | syspkg_pinned | `step: apt-get update &&  apt-get install -y --no-install-recommends   perl=5.32.1-4+deb11u5   perl-base=5.32.1-4+deb11u5   perl-modules-5.32=5.32.1-4+deb11u5...` |
| W0 | f4d-ponkotuy-myfleetgirls | 2 | syspkg_other | `step: apk add --no-cache openjdk8-jdk sbt` |
| W0 | f4d-ponkotuy-myfleetgirls | 3 | syspkg_other | `step: apk add --no-cache openjdk8-jre-headless curl &&     mkdir -p /root/bin &&     curl -s https://raw.githubusercontent.com/paulp/sbt-extras/master/sbt > ...` |
| W0 | f4d-appwish-graphqlapi | 3 | compile | `step: ./gradlew shadowJar --no-daemon` |
| W0 | f4d-hmbsbige-bilibilimangaautoclockin | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl=7.64.0-4+deb10u9         libcurl4=7.64.0-4+deb10u9         libtasn1-6=4.1...` |
| W0 | f4d-linux-china-mail-catcher | 1 | base_image | `ERROR: failed to build: failed to solve: maven:3.8.9-jdk-8-slim: failed to resolve source metadata for docker.io/library/maven:3.8.9-jdk-8-slim: docker.io/li...` |
| W0 | f4d-linux-china-mail-catcher | 2 | structure | `ERROR: failed to build: failed to solve: mkdir /usr/src/app: not a directory` |
| W0 | f4d-hmbsbige-bilibilimangaautoclockin | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl=7.64.0-4+deb10u9         libcurl4=7.64.0-4+deb10u9         libtasn1-6=4.1...` |
| W0 | f4d-hmbsbige-bilibilimangaautoclockin | 3 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl=7.64.0-4+deb10u9         libcurl4=7.64.0-4+deb10u9         libtasn1-6=4.1...` |
| W0 | f4d-ideatosrl-idephix | 1 | base_image | `ERROR: failed to build: failed to solve: php:7.1-cli-alpine3.12: failed to resolve source metadata for docker.io/library/php:7.1-cli-alpine3.12: docker.io/li...` |
| W0 | f4d-ideatosrl-idephix | 3 | base_image | `ERROR: failed to build: failed to solve: php:7.1-cli-alpine3.13: failed to resolve source metadata for docker.io/library/php:7.1-cli-alpine3.13: docker.io/li...` |
| W0 | f4d-couchrest-couchrest-model | 1 | structure | `ERROR: failed to build: failed to solve: failed to compute cache key: failed to calculate checksum of ref 4vokk8e9qg4g8mwb4kna3qp1q::qtboshazsi87h4zupr59y376...` |
| W0 | f4d-couchrest-couchrest-model | 2 | base_image | `ERROR: failed to build: failed to solve: ruby:2.3.9-slim: failed to resolve source metadata for docker.io/library/ruby:2.3.9-slim: docker.io/library/ruby:2.3...` |
| W0 | f4d-couchrest-couchrest-model | 3 | base_image | `ERROR: failed to build: failed to solve: ruby:2.3.10-buster: failed to resolve source metadata for docker.io/library/ruby:2.3.10-buster: docker.io/library/ru...` |
| W0 | f4d-minglecm-doc-to-dash | 3 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl=8.14.1-2+deb13u7         libcurl3t64-gnutls=8.14.1-2+deb13u7         libc...` |
| W0 | f4d-icemic-search-bakery-moe | 1 | lang_deps | `37.39 npm error A complete log of this run can be found in: /root/.npm/_logs/2026-10-20T23_54_14_075Z-debug-0.log` |
| W0 | f4d-icemic-search-bakery-moe | 2 | lang_deps | `57.17 npm error A complete log of this run can be found in: /root/.npm/_logs/2026-10-20T23_55_03_119Z-debug-0.log` |
| W0 | f4d-digimarc-corp-whalerator | 2 | lang_deps | `docker build: exit code 1: 8.277 npm error` |
| W0 | f4d-icemic-search-bakery-moe | 3 | lang_deps | `15.11 npm error A complete log of this run can be found in: /root/.npm/_logs/2026-10-20T23_56_05_256Z-debug-0.log` |
| W0 | f4d-digimarc-corp-whalerator | 3 | lang_deps | `docker build: exit code 1: 101.0 npm error` |
| W0 | f4d-flix-tech-avro-serde-php | 2 | base_image | `ERROR: failed to build: failed to solve: failed to parse stage name "php:8.1-cli-alpine:3.20": invalid reference format` |
| W0 | f4d-flix-tech-avro-serde-php | 3 | base_image | `ERROR: failed to build: failed to solve: failed to parse stage name "php:8.1-cli-alpine:3.21": invalid reference format` |
| W0 | f4d-flores-moarcats-app | 1 | lang_deps | `docker build: exit code 1: 88.47 An error occurred while installing nio4r (2.5.8), and Bundler cannot continue.` |
| W0 | f4d-dsibilio-rsocket-demo | 2 | syspkg_other | `1.254 E: Failed to fetch http://security.debian.org/debian-security/dists/stretch/updates/main/binary-amd64/Packages  404  Not Found` |
| W0 | f4d-flores-moarcats-app | 2 | lang_deps | `docker build: exit code 1: 100.9 An error occurred while installing nio4r (2.5.8), and Bundler cannot continue.` |
| W0 | f4d-bizanc-bizanc-io-core | 1 | syspkg_pinned | `step: apt-get update     && apt-get install -y --no-install-recommends         libc-bin=2.28-10+deb10u4         libc6=2.28-10+deb10u4         curl=7.64.0-4+d...` |
| W0 | f4d-flores-moarcats-app | 3 | lang_deps | `step: gem install -v 2.3.20 bundler && bundle install` |
| W0 | f4d-bizanc-bizanc-io-core | 3 | syspkg_pinned | `step: apt-get update &&     apt-get install --no-install-recommends --no-cache -y         libc-bin=2.28-10+deb10u4         libc6=2.28-10+deb10u4         curl...` |
| W0 | f4d-amiralies-metalarchives-api | 1 | syspkg_pinned | `step: apk add --no-cache --update     libcrypto1.1>=1.1.1l-r0     libssl1.1>=1.1.1l-r0     busybox>=1.31.1-r11     zlib>=1.2.11-r4     libgcc>=9.3.0-r0     l...` |
| W0 | f4d-ety001-online-clipboard | 1 | missing_tool | `exit code 127: cd /app && git clone https://github.com/ety001/online-clipboard.git &&     cd online-clipboard &&     git fetch origin fe &&     git checkout ...` |
| W0 | f4d-ety001-online-clipboard | 2 | missing_tool | `exit code 127: apk --no-cache add nodejs git &&     git clone https://github.com/ety001/online-clipboard.git &&     cd online-clipboard &&     git fetch orig...` |
| W0 | f4d-ety001-online-clipboard | 3 | missing_tool | `exit code 127: cd /app && git clone https://github.com/ety001/online-clipboard.git &&     cd online-clipboard &&     git fetch origin fe &&     git checkout ...` |
| W0 | f4d-atomheartother-wanwan-html5 | 1 | syspkg_pinned | `step: apk upgrade --no-cache &&     apk add --no-cache     libx11=1.8.14-r0     libexpat=2.9.0-r0     libxpm=3.5.20-r0     zlib=1.3.2-r1` |
| W0 | f4d-atomheartother-wanwan-html5 | 2 | syspkg_pinned | `step: apk add --no-cache --update     libx11=1.8.14-r0     libexpat=2.9.0-r0     libxpm=3.5.20-r0     zlib=1.3.2-r1` |
| W0 | f4d-ardalis-specification | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0 | f4d-ardalis-specification | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         perl-base=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         libperl5.32=5...` |
| W0 | f4d-ardalis-specification | 3 | base_image | `ERROR: failed to build: failed to solve: mcr.microsoft.com/dotnet/sdk:6.0.36-alpine3.16: failed to resolve source metadata for mcr.microsoft.com/dotnet/sdk:6...` |
| W0 | f4d-alexr1712-messenger-to-telegram | 1 | syspkg_pinned | `docker build: exit code 1: 0.845 ERROR: unable to select packages:` |
| W0 | f4d-mixcore-mix-core | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends     libperl5.32=5.32.1-4+deb11u5     perl=5.32.1-4+deb11u5     perl-base=5.32.1-4+deb1...` |
| W0 | f4d-mixcore-mix-core | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0 | f4d-atifaziz-linqpadless | 2 | base_image | `ERROR: failed to build: failed to solve: mcr.microsoft.com/dotnet/sdk:6.0.36-alpine3.16: failed to resolve source metadata for mcr.microsoft.com/dotnet/sdk:6...` |
| W0 | f4d-atifaziz-linqpadless | 3 | base_image | `ERROR: failed to build: failed to solve: mcr.microsoft.com/dotnet/sdk:6.0.426-alpine3.17: failed to resolve source metadata for mcr.microsoft.com/dotnet/sdk:...` |
| W0 | f4d-centerforopenscience-cas-overlay | 2 | lang_deps | `4.803 [ERROR] [Help 1] http://cwiki.apache.org/confluence/display/MAVEN/PluginResolutionException` |
| W0 | f4d-centerforopenscience-cas-overlay | 3 | base_image | `ERROR: failed to build: failed to solve: maven:3.8.9-eclipse-temurin-17: failed to resolve source metadata for docker.io/library/maven:3.8.9-eclipse-temurin-...` |
| W0 | f4d-code4sac-codeforsacramento-org | 1 | base_image | `ERROR: failed to build: failed to solve: ruby:2.5-bullseye: failed to resolve source metadata for docker.io/library/ruby:2.5-bullseye: docker.io/library/ruby...` |
| W0 | f4d-code4sac-codeforsacramento-org | 2 | lang_deps | `15.99 ERROR:  Error installing bundler:` |
| W0 | f4d-18f-content-guide | 2 | unclear | `step: apt-get update &&     apt-get install -y --no-install-recommends         imagemagick=8:6.9.11.60+dfsg-1.3+deb11u16         linux-libc-dev=5.10.262-1   ...` |
| W0 | f4d-18f-content-guide | 3 | base_image | `ERROR: failed to build: failed to solve: ruby:2.7-bullseye-slim: failed to resolve source metadata for docker.io/library/ruby:2.7-bullseye-slim: docker.io/li...` |
| W0 | d3-raywangqvq-bilibilitoolpro | 1 | base_image | `ERROR: failed to build: failed to solve: failed to parse stage name "mcr.microsoft.com/dotnet/sdk:8.0.400@sha256:2e0a5a1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f...` |
| W0 | d3-raywangqvq-bilibilitoolpro | 2 | syspkg_pinned | `7.498 E: Version '5.36.0-7+deb12u4' for 'perl-base' was not found` |
| W0+W1 | demo-django | 1 | lang_deps | `2.072 ERROR: ResolutionImpossible: for help visit https://pip.pypa.io/en/latest/topics/dependency-resolution/#dealing-with-dependency-conflicts` |
| W0+W1 | demo-apache-php | 1 | syspkg_pinned | `4.436 E: Failed to fetch http://security.debian.org/debian-security/pool/updates/main/o/openssl/openssl_1.1.1w-0%2bdeb11u8_arm64.deb  404  Not Found [IP: 151...` |
| W0+W1 | demo-nodejs-web | 1 | syspkg_pinned | `step: apk add --no-cache     tar=7.5.21-r0     openssl=1.1.1t-r2     zlib=1.2.12-r2     apk-tools=2.12.6-r0     busybox=1.33.1-r7     ssl_client=1.33.1-r7   ...` |
| W0+W1 | demo-nodejs-web | 3 | syspkg_pinned | `step: apk add --no-cache     tar=7.5.21-r0     openssl=3.1.4-r0     zlib=1.2.13-r0     apk-tools=2.14.0-r0     busybox=1.36.1-r13` |
| W0+W1 | demo-aspnet | 1 | syspkg_pinned | `step: apt-get update && apt-get install -y --no-install-recommends     libc-bin=2.28-10+deb10u4     libc6=2.28-10+deb10u4     zlib1g=1:1.2.11.dfsg-1+deb10u2 ...` |
| W0+W1 | demo-aspnet | 2 | syspkg_pinned | `step: apt-get update && apt-get install -y --no-install-recommends     libc-bin=2.28-10+deb10u4     libc6=2.28-10+deb10u4     zlib1g=1:1.2.11.dfsg-1+deb10u2 ...` |
| W0+W1 | d3-glimesh-broadcast-box | 1 | syspkg_pinned | `2.002 ERROR: unable to select packages:` |
| W0+W1 | d3-glimesh-broadcast-box | 2 | syspkg_pinned | `1.940 ERROR: unable to select packages:` |
| W0+W1 | d3-glimesh-broadcast-box | 3 | missing_tool | `8.547 sh: vite: not found` |
| W0+W1 | d3-jfryy-qq | 1 | base_image | `ERROR: failed to build: failed to solve: golang:1.26-bullseye: failed to resolve source metadata for docker.io/library/golang:1.26-bullseye: docker.io/librar...` |
| W0+W1 | d3-jfryy-qq | 2 | lang_deps | `step: go get golang.org/x/net@v0.60.0 golang.org/x/text@v0.41.0 golang.org/x/sys@v0.44.0 && go mod tidy` |
| W0+W1 | d3-jfryy-qq | 3 | unclear | `step: go get golang.org/x/net@v0.60.0 golang.org/x/text@v0.41.0 golang.org/x/sys@v0.44.0 && go mod tidy && go build -o bin/qq -ldflags="-linkmode external -e...` |
| W0+W1 | d3-tracecathq-tracecat-prod | 3 | compile | `112.1 src/components/tags-input.tsx(19,6): error TS2739: Type '{ activeTagIndex: number \| null; setActiveTagIndex: Dispatch<SetStateAction<number \| null>>;...` |
| W0+W1 | d3-marcopiovanello-yt-dlp-web-ui | 3 | compile | `step: CGO_ENABLED=0 GOOS=linux go build -o yt-dlp-webui` |
| W0+W1 | d3-vogler-free-games-claimer | 1 | syspkg_pinned | `step: apt-get update     && apt-get install --no-install-recommends -y curl ca-certificates gnupg     && mkdir -p /etc/apt/keyrings     && curl -fsSL https:/...` |
| W0+W1 | f4d-fsprojects-argu | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0+W1 | f4d-fsprojects-argu | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0+W1 | f4d-jeffreyksmithjr-galapagos-nao | 3 | syspkg_other | `docker build: exit code 1: 2.135 E: The repository 'http://deb.debian.org/debian stretch Release' does no longer have a Release file.` |
| W0+W1 | f4d-fsprojects-argu | 3 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0+W1 | f4d-3scale-3scale-toolbox | 2 | lang_deps | `step: gem install bundler --version 2.3.5 --no-document &&     gem install nokogiri -v '>= 1.19.4' --no-document &&     gem install concurrent-ruby -v '>= 1....` |
| W0+W1 | f4d-wyhaya-updns | 2 | base_image | `ERROR: failed to build: failed to solve: rust:1.27.2-buster: failed to resolve source metadata for docker.io/library/rust:1.27.2-buster: docker.io/library/ru...` |
| W0+W1 | f4d-3scale-3scale-toolbox | 3 | unclear | `step: dnf upgrade --security -y environment-modules &&     dnf clean all &&     gem install bundler --version 2.3.5 --no-document &&     gem install nokogiri...` |
| W0+W1 | f4d-wyhaya-updns | 3 | compile | `step: cargo build --release` |
| W0+W1 | f4d-robinmonjo-coincoin | 1 | lang_deps | `step: mix local.hex --force && mix deps.get` |
| W0+W1 | f4d-robinmonjo-coincoin | 2 | compile | `step: make release` |
| W0+W1 | f4d-robinmonjo-coincoin | 3 | compile | `step: make release` |
| W0+W1 | f4d-eiriktsarpalis-typeshape | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0+W1 | d3-vogler-free-games-claimer | 3 | lang_deps | `step: ln -s /usr/share/novnc/vnc_auto.html /usr/share/novnc/index.html     && pip install --no-cache-dir apprise` |
| W0+W1 | f4d-eiriktsarpalis-typeshape | 3 | syspkg_pinned | `step: apt-get update &&  apt-get install -y --no-install-recommends   perl=5.32.1-4+deb11u5   perl-base=5.32.1-4+deb11u5   perl-modules-5.32=5.32.1-4+deb11u5...` |
| W0+W1 | f4d-appwish-graphqlapi | 1 | compile | `step: ./gradlew shadowJar` |
| W0+W1 | f4d-couchrest-couchrest-model | 1 | structure | `ERROR: failed to build: failed to solve: failed to compute cache key: failed to calculate checksum of ref 4vokk8e9qg4g8mwb4kna3qp1q::ou7x634ludea5uj6sxgvhd7w...` |
| W0+W1 | f4d-couchrest-couchrest-model | 2 | lang_deps | `step: gem install bundler -v '>= 2.2.33' --no-document &&     gem install nokogiri -v '>= 1.19.4' --no-document &&     gem install json -v '>= 2.3.0' --no-do...` |
| W0+W1 | f4d-couchrest-couchrest-model | 3 | base_image | `ERROR: failed to build: failed to solve: ruby:2.3.10: failed to resolve source metadata for docker.io/library/ruby:2.3.10: docker.io/library/ruby:2.3.10: not...` |
| W0+W1 | f4d-hmbsbige-bilibilimangaautoclockin | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl=7.64.0-4+deb10u9         libcurl4=7.64.0-4+deb10u9         libtasn1-6=4.1...` |
| W0+W1 | f4d-hmbsbige-bilibilimangaautoclockin | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl=7.64.0-4+deb10u9         libcurl4=7.64.0-4+deb10u9         libtasn1-6=4.1...` |
| W0+W1 | f4d-hmbsbige-bilibilimangaautoclockin | 3 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl=7.64.0-4+deb10u9         libcurl4=7.64.0-4+deb10u9         libtasn1-6=4.1...` |
| W0+W1 | f4d-linux-china-mail-catcher | 2 | structure | `ERROR: failed to build: failed to solve: mkdir /usr/src/app: not a directory` |
| W0+W1 | f4d-minglecm-doc-to-dash | 3 | syspkg_other | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl         libcurl3t64-gnutls         libcurl4-openssl-dev         libcurl4t...` |
| W0+W1 | f4d-digimarc-corp-whalerator | 2 | lang_deps | `docker build: exit code 1: 10.36 npm error` |
| W0+W1 | f4d-icemic-search-bakery-moe | 1 | lang_deps | `19.73 npm error A complete log of this run can be found in: /root/.npm/_logs/2026-10-21T02_01_28_871Z-debug-0.log` |
| W0+W1 | f4d-icemic-search-bakery-moe | 2 | lang_deps | `74.42 npm error A complete log of this run can be found in: /root/.npm/_logs/2026-10-21T02_02_03_363Z-debug-0.log` |
| W0+W1 | f4d-icemic-search-bakery-moe | 3 | lang_deps | `31.19 npm error A complete log of this run can be found in: /root/.npm/_logs/2026-10-21T02_03_36_077Z-debug-0.log` |
| W0+W1 | f4d-digimarc-corp-whalerator | 3 | lang_deps | `docker build: exit code 1: 154.2 npm error` |
| W0+W1 | f4d-dsibilio-rsocket-demo | 2 | syspkg_other | `1.281 E: Failed to fetch http://security.debian.org/debian-security/dists/stretch/updates/main/binary-amd64/Packages  404  Not Found` |
| W0+W1 | f4d-flores-moarcats-app | 1 | lang_deps | `docker build: exit code 1: 104.6 An error occurred while installing nio4r (2.5.8), and Bundler cannot continue.` |
| W0+W1 | f4d-flores-moarcats-app | 2 | lang_deps | `docker build: exit code 1: 112.1 An error occurred while installing nio4r (2.5.8), and Bundler cannot continue.` |
| W0+W1 | f4d-flores-moarcats-app | 3 | lang_deps | `step: gem install -v 2.3.20 bundler && bundle install` |
| W0+W1 | f4d-bizanc-bizanc-io-core | 1 | syspkg_pinned | `step: apt-get update     && apt-get install -y --no-install-recommends         libc-bin=2.28-10+deb10u4         libc6=2.28-10+deb10u4         curl=7.64.0-4+d...` |
| W0+W1 | f4d-flix-tech-avro-serde-php | 3 | unclear | `step: apk add --no-cache --virtual .build-deps $PHPIZE_DEPS     && apk add --no-cache --virtual .runtime-deps git libzip-dev     && docker-php-source extract...` |
| W0+W1 | f4d-amiralies-metalarchives-api | 1 | syspkg_pinned | `step: apk add --no-cache --update     libcrypto1.1=1.1.1l-r0     libssl1.1=1.1.1l-r0     busybox=1.31.1-r11     zlib=1.2.11-r4     libgcc=9.3.0-r0     libstd...` |
| W0+W1 | f4d-bizanc-bizanc-io-core | 3 | syspkg_pinned | `step: apt-get update &&     apt-get install --no-install-recommends --no-cache -y         libc-bin=2.28-10+deb10u4         libc6=2.28-10+deb10u4         curl...` |
| W0+W1 | f4d-ety001-online-clipboard | 1 | missing_tool | `exit code 127: cd /app && git clone https://github.com/ety001/online-clipboard.git &&     cd online-clipboard &&     git fetch origin fe &&     git checkout ...` |
| W0+W1 | f4d-ety001-online-clipboard | 2 | missing_tool | `exit code 127: apk --no-cache add nodejs git &&     git clone https://github.com/ety001/online-clipboard.git &&     cd online-clipboard &&     git fetch orig...` |
| W0+W1 | f4d-ety001-online-clipboard | 3 | missing_tool | `exit code 127: cd /app && git clone https://github.com/ety001/online-clipboard.git &&     cd online-clipboard &&     git fetch origin fe &&     git checkout ...` |
| W0+W1 | f4d-atomheartother-wanwan-html5 | 1 | syspkg_pinned | `step: apk add --no-cache     libx11=1.8.14-r0     libexpat=2.9.0-r0     libxpm=3.5.20-r0     zlib=1.3.2-r1` |
| W0+W1 | f4d-atomheartother-wanwan-html5 | 2 | syspkg_pinned | `step: apk add --no-cache --update     libx11=1.8.14-r0     libexpat=2.9.0-r0     libxpm=3.5.20-r0     zlib=1.3.2-r1` |
| W0+W1 | f4d-ardalis-specification | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0+W1 | f4d-mixcore-mix-core | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends     libperl5.32=5.32.1-4+deb11u5     perl=5.32.1-4+deb11u5     perl-base=5.32.1-4+deb1...` |
| W0+W1 | f4d-mixcore-mix-core | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0+W1 | f4d-ardalis-specification | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         perl-base=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         libperl5.32=5...` |
| W0+W1 | f4d-ardalis-specification | 3 | missing_tool | `exit code 127: apk add --no-cache bash=5.1.16-r0 curl=7.79.1-r4` |
| W0+W1 | f4d-alexr1712-messenger-to-telegram | 1 | syspkg_pinned | `docker build: exit code 1: unable to select packages:` |
| W0+W1 | f4d-centerforopenscience-cas-overlay | 2 | lang_deps | `27.97 [ERROR] [Help 1] http://cwiki.apache.org/confluence/display/MAVEN/PluginResolutionException` |
| W0+W1 | f4d-alexr1712-messenger-to-telegram | 2 | syspkg_pinned | `unable to select packages:` |
| W0+W1 | f4d-atifaziz-linqpadless | 2 | base_image | `ERROR: failed to build: failed to solve: mcr.microsoft.com/dotnet/sdk:6.0.36-alpine3.16: failed to resolve source metadata for mcr.microsoft.com/dotnet/sdk:6...` |
| W0+W1 | f4d-atifaziz-linqpadless | 3 | base_image | `ERROR: failed to build: failed to solve: mcr.microsoft.com/dotnet/sdk:6.0.426: failed to resolve source metadata for mcr.microsoft.com/dotnet/sdk:6.0.426: mc...` |
| W0+W1 | f4d-18f-content-guide | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         imagemagick=8:6.9.11.60+dfsg-1.3+deb11u16         imagemagick-6-common=8:6.9.1...` |
| W0+W1 | f4d-18f-content-guide | 2 | unclear | `step: apt-get update &&     apt-get install -y --no-install-recommends         imagemagick=8:6.9.11.60+dfsg-1.3+deb11u16         linux-libc-dev=5.10.262-1   ...` |
| W0+W1 | f4d-18f-content-guide | 3 | unclear | `step: apt-get update &&     apt-get install -y --no-install-recommends     imagemagick=8:6.9.11.60+dfsg-1.3+deb11u16     linux-libc-dev=5.10.262-1     libaom...` |
| W0+W1 | f4d-code4sac-codeforsacramento-org | 2 | lang_deps | `21.62 ERROR:  Error installing bundler:` |
| W0+W1+W2 | demo-fastapi | 1 | structure | `ERROR: failed to build: failed to solve: target stage "builder" could not be found` |
| W0+W1+W2 | demo-aspnet | 1 | syspkg_pinned | `step: apt-get update && apt-get -y upgrade --only-upgrade     libc-bin=2.28-10+deb10u4     libc6=2.28-10+deb10u4     zlib1g=1:1.2.11.dfsg-1+deb10u2     libta...` |
| W0+W1+W2 | demo-aspnet | 2 | syspkg_pinned | `step: apt-get update && apt-get install -y --no-install-recommends     libc-bin=2.28-10+deb10u4     libc6=2.28-10+deb10u4     zlib1g=1:1.2.11.dfsg-1+deb10u2 ...` |
| W0+W1+W2 | d3-glimesh-broadcast-box | 1 | missing_tool | `0.260 /bin/sh: npm: not found` |
| W0+W1+W2 | d3-glimesh-broadcast-box | 2 | missing_tool | `0.104 /bin/sh: npm: not found` |
| W0+W1+W2 | d3-jfryy-qq | 1 | base_image | `ERROR: failed to build: failed to solve: golang:1.26-bullseye: failed to resolve source metadata for docker.io/library/golang:1.26-bullseye: docker.io/librar...` |
| W0+W1+W2 | d3-glimesh-broadcast-box | 3 | unclear | `step: apk add --no-cache git &&     go get golang.org/x/crypto@v0.56.0 golang.org/x/net@v0.60.0 github.com/pion/interceptor@v0.1.39 github.com/pion/dtls/v3@v...` |
| W0+W1+W2 | d3-jfryy-qq | 2 | lang_deps | `step: go get golang.org/x/net@v0.60.0 golang.org/x/text@v0.41.0 golang.org/x/sys@v0.44.0 && go mod tidy` |
| W0+W1+W2 | d3-jfryy-qq | 3 | unclear | `step: go get golang.org/x/net@v0.60.0 golang.org/x/text@v0.41.0 golang.org/x/sys@v0.44.0 && go mod tidy && go build -o bin/qq -ldflags="-linkmode external -e...` |
| W0+W1+W2 | d3-astrbotdevs-astrbot | 1 | lang_deps | `0.650 error: Failed to parse: `mcp>=1.30.0,2.2.0`` |
| W0+W1+W2 | d3-bruin-data-bruin | 1 | syspkg_other | `step: apt-get update && apt-get install -y --no-cache curl ca-certificates && rm -rf /var/lib/apt/lists/*` |
| W0+W1+W2 | d3-bruin-data-bruin | 2 | missing_tool | `exit code 127: adduser --disabled-password --gecos '' bruin` |
| W0+W1+W2 | d3-bruin-data-bruin | 3 | missing_tool | `exit code 127: adduser --disabled-password --gecos '' bruin` |
| W0+W1+W2 | d3-astrbotdevs-astrbot | 2 | unclear | `step: apt-get update && apt-get install -y --no-install-recommends     nodejs     npm     gcc     build-essential     python3-dev     libffi-dev     libssl-d...` |
| W0+W1+W2 | d3-tracecathq-tracecat-prod | 3 | compile | `104.5 src/components/tags-input.tsx(19,6): error TS2739: Type '{ activeTagIndex: number \| null; setActiveTagIndex: Dispatch<SetStateAction<number \| null>>;...` |
| W0+W1+W2 | d3-marcopiovanello-yt-dlp-web-ui | 1 | unclear | `step: apk add --no-cache ffmpeg ca-certificates curl wget gnutls &&     pip install --no-cache-dir "yt-dlp[default,curl-cffi,mutagen,pycryptodomex,phantomjs,...` |
| W0+W1+W2 | d3-projectdiscovery-httpx | 1 | lang_deps | `step: go mod download     && go get stdlib@1.26.9     && go get golang.org/x/crypto@0.56.0     && go get golang.org/x/net@0.60.0     && go get golang.org/x/t...` |
| W0+W1+W2 | d3-projectdiscovery-httpx | 2 | lang_deps | `step: go mod edit -require=golang.org/x/crypto@v0.56.0 -require=golang.org/x/net@v0.60.0 -require=golang.org/x/text@v0.41.0 -require=golang.org/x/oauth2@v0.2...` |
| W0+W1+W2 | d3-projectdiscovery-httpx | 3 | lang_deps | `docker build: exit code 1: 5.986 go: github.com/go-viper/mapstructure/v2@2.4.0: invalid version: unknown revision 2.4.0` |
| W0+W1+W2 | d3-vogler-free-games-claimer | 1 | syspkg_other | `step: apt-get update     && apt-get install --no-install-recommends -y curl ca-certificates gnupg     && mkdir -p /etc/apt/keyrings     && curl -fsSL https:/...` |
| W0+W1+W2 | f4d-jeffreyksmithjr-galapagos-nao | 2 | syspkg_pinned | `step: apt-get update && apt-get install -y --no-install-recommends         ca-certificates=20200601~deb9u2         curl=7.52.1-5+deb9u16         git=1:2.11.0...` |
| W0+W1+W2 | f4d-jeffreyksmithjr-galapagos-nao | 3 | syspkg_other | `docker build: exit code 1: 2.370 E: The repository 'http://security.debian.org stretch/updates Release' does no longer have a Release file.` |
| W0+W1+W2 | f4d-fsprojects-argu | 1 | syspkg_pinned | `step: apt-get update &&  apt-get install -y --no-install-recommends   libperl5.32=5.32.1-4+deb11u5   perl=5.32.1-4+deb11u5   perl-base=5.32.1-4+deb11u5   per...` |
| W0+W1+W2 | f4d-fsprojects-argu | 2 | syspkg_pinned | `step: apt-get update &&  apt-get install -y --no-install-recommends   libperl5.32=5.32.1-4+deb11u5   perl=5.32.1-4+deb11u5   perl-base=5.32.1-4+deb11u5   per...` |
| W0+W1+W2 | f4d-fsprojects-argu | 3 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0+W1+W2 | d3-vogler-free-games-claimer | 2 | syspkg_other | `step: apt-get update     && apt-get install --no-install-recommends -y curl ca-certificates gnupg     && mkdir -p /etc/apt/keyrings     && curl -fsSL https:/...` |
| W0+W1+W2 | f4d-3scale-3scale-toolbox | 2 | lang_deps | `step: gem install bundler --version 2.3.5 --no-document &&     gem install nokogiri -v '>= 1.19.4' --no-document &&     gem install concurrent-ruby -v '>= 1....` |
| W0+W1+W2 | f4d-wyhaya-updns | 2 | compile | `step: cargo build --release` |
| W0+W1+W2 | f4d-3scale-3scale-toolbox | 3 | unclear | `step: dnf upgrade --security -y environment-modules &&     dnf clean all &&     gem install bundler --version 2.3.5 --no-document &&     gem install nokogiri...` |
| W0+W1+W2 | f4d-wyhaya-updns | 3 | compile | `step: cargo build --release` |
| W0+W1+W2 | f4d-robinmonjo-coincoin | 1 | compile | `step: make release` |
| W0+W1+W2 | f4d-robinmonjo-coincoin | 2 | compile | `step: make release` |
| W0+W1+W2 | f4d-robinmonjo-coincoin | 3 | compile | `step: make release` |
| W0+W1+W2 | d3-vogler-free-games-claimer | 3 | lang_deps | `step: pip install --no-cache-dir apprise` |
| W0+W1+W2 | f4d-couchrest-couchrest-model | 1 | lang_deps | `step: bundle install --jobs=3 --retry=3` |
| W0+W1+W2 | f4d-couchrest-couchrest-model | 3 | lang_deps | `step: gem install bundler -v '>= 2.2.33' &&     gem install nokogiri -v '>= 1.19.4' &&     gem install rake -v '>= 12.3.3' &&     gem install json -v '>= 2.3...` |
| W0+W1+W2 | f4d-hmbsbige-bilibilimangaautoclockin | 1 | syspkg_other | `step: apt-get update &&     apt-get upgrade -y --no-install-recommends &&     apt-get clean &&     rm -rf /var/lib/apt/lists/*` |
| W0+W1+W2 | f4d-hmbsbige-bilibilimangaautoclockin | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl=7.64.0-4+deb10u9         libcurl4=7.64.0-4+deb10u9         libtasn1-6=4.1...` |
| W0+W1+W2 | f4d-appwish-graphqlapi | 3 | compile | `step: ./gradlew shadowJar` |
| W0+W1+W2 | f4d-ideatosrl-idephix | 1 | syspkg_pinned | `step: apk add --no-cache     git     curl=7.66.0-r4     libcurl=7.66.0-r4     libssl1.1=1.1.1k-r0     libcrypto1.1=1.1.1k-r0     openssl=1.1.1k-r0     libxml...` |
| W0+W1+W2 | f4d-hmbsbige-bilibilimangaautoclockin | 3 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         curl=7.64.0-4+deb10u9         libcurl4=7.64.0-4+deb10u9         libtasn1-6=4.1...` |
| W0+W1+W2 | f4d-ideatosrl-idephix | 3 | syspkg_pinned | `step: apk add --no-cache     apk-tools=2.10.7-r0     curl=7.66.0-r4     libcurl=7.66.0-r4     libcrypto1.1=1.1.1k-r0     libssl1.1=1.1.1k-r0     openssl=1.1....` |
| W0+W1+W2 | f4d-linux-china-mail-catcher | 2 | structure | `ERROR: failed to build: failed to solve: mkdir /usr/src/app: not a directory` |
| W0+W1+W2 | f4d-digimarc-corp-whalerator | 2 | lang_deps | `docker build: exit code 1: 8.092 npm error` |
| W0+W1+W2 | f4d-digimarc-corp-whalerator | 3 | lang_deps | `docker build: exit code 1: 154.2 npm error` |
| W0+W1+W2 | f4d-icemic-search-bakery-moe | 2 | lang_deps | `docker build: exit code 1: 45.83 npm ERR! Failed at the nodejieba@2.4.1 install script.` |
| W0+W1+W2 | f4d-flix-tech-avro-serde-php | 1 | unclear | `step: apk add --no-cache --virtual .build-deps $PHPIZE_DEPS     && apk add --no-cache --virtual .runtime-deps git libzip-dev     && docker-php-source extract...` |
| W0+W1+W2 | f4d-flix-tech-avro-serde-php | 2 | unclear | `step: apk add --no-cache --virtual .build-deps $PHPIZE_DEPS     && apk add --no-cache --virtual .runtime-deps git libzip-dev     && docker-php-source extract...` |
| W0+W1+W2 | f4d-flores-moarcats-app | 1 | lang_deps | `docker build: exit code 1: 116.1 An error occurred while installing nio4r (2.5.8), and Bundler cannot continue.` |
| W0+W1+W2 | f4d-flix-tech-avro-serde-php | 3 | unclear | `step: apk add --no-cache --virtual .build-deps $PHPIZE_DEPS     && apk add --no-cache --virtual .runtime-deps git libzip-dev     && docker-php-source extract...` |
| W0+W1+W2 | f4d-centerforopenscience-cas-overlay | 1 | compile | `step: mvn clean package -P nocheck` |
| W0+W1+W2 | f4d-flores-moarcats-app | 2 | lang_deps | `docker build: exit code 1: 127.9 An error occurred while installing nio4r (2.5.8), and Bundler cannot continue.` |
| W0+W1+W2 | f4d-flores-moarcats-app | 3 | lang_deps | `step: gem install -v 2.3.20 bundler && bundle install` |
| W0+W1+W2 | f4d-amiralies-metalarchives-api | 1 | syspkg_pinned | `step: apk add --no-cache --update     libcrypto1.1=1.1.1l-r0     libssl1.1=1.1.1l-r0     busybox=1.31.1-r11     zlib=1.2.11-r4     libgcc=9.3.0-r0     libstd...` |
| W0+W1+W2 | f4d-ety001-online-clipboard | 1 | missing_tool | `exit code 127: cd /app && git clone https://github.com/ety001/online-clipboard.git &&     cd online-clipboard &&     git fetch origin fe &&     git checkout ...` |
| W0+W1+W2 | f4d-ety001-online-clipboard | 2 | missing_tool | `exit code 127: apk --no-cache add nodejs git &&     git clone https://github.com/ety001/online-clipboard.git &&     cd online-clipboard &&     git fetch orig...` |
| W0+W1+W2 | f4d-centerforopenscience-cas-overlay | 2 | compile | `step: mvn clean package -P nocheck` |
| W0+W1+W2 | f4d-ety001-online-clipboard | 3 | missing_tool | `exit code 127: cd /app && git clone https://github.com/ety001/online-clipboard.git &&     cd online-clipboard &&     git fetch origin fe &&     git checkout ...` |
| W0+W1+W2 | f4d-ardalis-specification | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0+W1+W2 | f4d-centerforopenscience-cas-overlay | 3 | compile | `step: mvn clean package -P nocheck` |
| W0+W1+W2 | f4d-ardalis-specification | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         perl-base=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         libperl5.32=5...` |
| W0+W1+W2 | f4d-ardalis-specification | 3 | missing_tool | `exit code 127: apk add --no-cache bash=5.1.16-r0 curl=7.79.1-r4` |
| W0+W1+W2 | f4d-mixcore-mix-core | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends     libperl5.32=5.32.1-4+deb11u5     perl=5.32.1-4+deb11u5     perl-base=5.32.1-4+deb1...` |
| W0+W1+W2 | f4d-atifaziz-linqpadless | 1 | compile | `step: dotnet pack -c Release src` |
| W0+W1+W2 | f4d-atifaziz-linqpadless | 2 | compile | `step: dotnet pack -c Release src` |
| W0+W1+W2 | f4d-mixcore-mix-core | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5         perl=5.32.1-4+deb11u5         perl-base=5...` |
| W0+W1+W2 | f4d-mixcore-mix-core | 3 | syspkg_pinned | `step: apt-get update &&     apt-get upgrade -y --no-install-recommends &&     apt-get install -y --no-install-recommends         libperl5.32=5.32.1-4+deb11u5...` |
| W0+W1+W2 | f4d-code4sac-codeforsacramento-org | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends     libexpat1=2.2.6-2+deb10u7     libexpat1-dev=2.2.6-2+deb10u7     libc-bin=2.28-10+d...` |
| W0+W1+W2 | f4d-18f-content-guide | 1 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         imagemagick=8:6.9.11.60+dfsg-1.3+deb11u16         imagemagick-6-common=8:6.9.1...` |
| W0+W1+W2 | f4d-18f-content-guide | 2 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends         imagemagick=8:6.9.11.60+dfsg-1.3+deb11u16         linux-libc-dev=5.10.262-1   ...` |
| W0+W1+W2 | f4d-code4sac-codeforsacramento-org | 3 | syspkg_pinned | `step: apt-get update && apt-get install -y --no-install-recommends     libexpat1=2.2.6-2+deb10u7     libexpat1-dev=2.2.6-2+deb10u7     libc-bin=2.28-10+deb10...` |
| W0+W1+W2 | f4d-18f-content-guide | 3 | syspkg_pinned | `step: apt-get update &&     apt-get install -y --no-install-recommends     imagemagick=8:6.9.11.60+dfsg-1.3+deb11u16     linux-libc-dev=5.10.262-1     libaom...` |
| Baseline | f4d-robinmonjo-coincoin | 1 | compile | `step: make release` |
| Baseline | f4d-flix-tech-avro-serde-php | 1 | unclear | `step: apk add --no-cache --virtual .build-deps $PHPIZE_DEPS     && apk add --no-cache --virtual .runtime-deps git libzip-dev     && docker-php-source extract...` |
| Baseline | f4d-centerforopenscience-cas-overlay | 1 | compile | `step: mvn clean package -P nocheck` |
