FROM mcr.microsoft.com/dotnet/core/sdk:3.1.101 AS build-env
WORKDIR /app

COPY ./. ./

RUN dotnet restore Bizanc.io.Matching.sln \
    && mkdir -p out \
    && dotnet publish Bizanc.io.Matching.App/Bizanc.io.Matching.App.csproj -c Release -o ./out

FROM mcr.microsoft.com/dotnet/core/aspnet:3.1.1
WORKDIR /app

COPY --from=build-env /app/out ./

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        libc-bin=2.28-10+deb10u4 \
        libc6=2.28-10+deb10u4 \
        curl=7.64.0-4+deb10u9 \
        libcurl4=7.64.0-4+deb10u9 \
        libgnutls30=3.6.7-4+deb10u12 \
        zlib1g=1:1.2.11.dfsg-1+deb10u2 \
        libldap-2.4-2=2.4.47+dfsg-3+deb10u7 \
        libldap-common=2.4.47+dfsg-3+deb10u7 \
        libssl1.1=1.1.1n-0+deb10u6 \
        openssl=1.1.1n-0+deb10u6 \
        libidn2-0=2.0.5-1+deb10u1 \
        liblz4-1=1.8.3-1+deb10u1 \
        dpkg=1.19.8 \
        libtasn1-6=4.13-3+deb10u1 \
        libsystemd0=241-7~deb10u10 \
        libudev1=241-7~deb10u10 \
        perl-base=5.28.1-6+deb10u1 \
        libgssapi-krb5-2=1.17-3+deb10u6 \
        libk5crypto3=1.17-3+deb10u6 \
        libkrb5-3=1.17-3+deb10u6 \
        libkrb5support0=1.17-3+deb10u6 \
        libncursesw6=6.1+20181013-2+deb10u5 \
        libtinfo6=6.1+20181013-2+deb10u5 \
        ncurses-base=6.1+20181013-2+deb10u5 \
        ncurses-bin=6.1+20181013-2+deb10u5 \
        libssh2-1=1.8.0-2.1+deb10u1 \
        libnghttp2-14=1.36.0-2+deb10u3 \
        libp11-kit0=0.23.15-2+deb10u1 \
        libhogweed4=3.4.1-1+deb10u1 \
        libnettle6=3.4.1-1+deb10u1 \
        libgcrypt20=1.8.4-5+deb10u1 \
        e2fsprogs=1.44.5-1+deb10u3 \
        libcom-err2=1.44.5-1+deb10u3 \
        libext2fs2=1.44.5-1+deb10u3 \
        libicu63=63.1-6+deb10u2 \
        libss2=1.44.5-1+deb10u3 \
        gzip=1.9-3+deb10u1 \
        libgmp10=2:6.1.2+dfsg-4+deb10u1 \
        liblzma5=5.2.4-1+deb10u1 \
        libsasl2-2=2.1.27+dfsg-1+deb10u2 \
        libsasl2-modules-db=2.1.27+dfsg-1+deb10u2 \
        apt=1.8.2.2 \
        libapt-pkg5.0=1.8.2.2 \
        libzstd1=1.3.8+dfsg-3+deb10u2 \
        bsdutils=2.33.1-0.1+deb10u1 \
        fdisk=2.33.1-0.1+deb10u1 \
        gpgv=2.2.12-1+deb10u2 \
        libblkid1=2.33.1-0.1+deb10u1 \
        libfdisk1=2.33.1-0.1+deb10u1 \
        libmount1=2.33.1-0.1+deb10u1 \
        libsmartcols1=2.33.1-0.1+deb10u1 \
        libuuid1=2.33.1-0.1+deb10u1 \
        mount=2.33.1-0.1+deb10u1 \
        util-linux=2.33.1-0.1+deb10u1 \
        tar=1.30+dfsg-6+deb10u1 \
        tzdata=2024a-0+deb10u1 \
        debian-archive-keyring=2019.1+deb10u2 \
        libbz2-1.0=1.0.6-9.2~deb10u2 \
    && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["dotnet", "Bizanc.io.Matching.App.dll"]
