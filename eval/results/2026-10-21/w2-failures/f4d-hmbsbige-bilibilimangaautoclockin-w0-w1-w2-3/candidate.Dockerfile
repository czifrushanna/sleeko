FROM mcr.microsoft.com/dotnet/core/runtime:3.1-buster-slim AS base
WORKDIR /app

FROM mcr.microsoft.com/dotnet/core/sdk:3.1-buster AS build
WORKDIR /src
COPY ["BilibiliMangaAutoClockIn/BilibiliMangaAutoClockIn.csproj", "BilibiliMangaAutoClockIn/"]
COPY ["BilibiliApi/BilibiliApi.csproj", "BilibiliApi/"]
RUN dotnet restore "BilibiliMangaAutoClockIn/BilibiliMangaAutoClockIn.csproj"
COPY . .
WORKDIR "/src/BilibiliMangaAutoClockIn"
RUN dotnet build "BilibiliMangaAutoClockIn.csproj" -c Release -o /app/build

FROM build AS publish
RUN dotnet publish "BilibiliMangaAutoClockIn.csproj" -c Release -o /app/publish

FROM mcr.microsoft.com/dotnet/core/runtime:3.1-buster-slim AS final
WORKDIR /app
COPY --from=publish /app/publish .
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        curl=7.64.0-4+deb10u9 \
        libcurl4=7.64.0-4+deb10u9 \
        libtasn1-6=4.13-3+deb10u1 \
        libsystemd0=241-7~deb10u10 \
        libudev1=241-7~deb10u10 \
        libssl1.1=1.1.1n-0+deb10u6 \
        openssl=1.1.1n-0+deb10u6 \
        libc-bin=2.28-10+deb10u4 \
        libc6=2.28-10+deb10u4 \
        libssh2-1=1.8.0-2.1+deb10u1 \
        libncursesw6=6.1+20181013-2+deb10u5 \
        libtinfo6=6.1+20181013-2+deb10u5 \
        ncurses-base=6.1+20181013-2+deb10u5 \
        ncurses-bin=6.1+20181013-2+deb10u5 \
        libgnutls30=3.6.7-4+deb10u12 \
        libnghttp2-14=1.36.0-2+deb10u3 \
        bsdutils=1:2.33.1-0.1+deb10u1 \
        fdisk=2.33.1-0.1+deb10u1 \
        libblkid1=2.33.1-0.1+deb10u1 \
        libfdisk1=2.33.1-0.1+deb10u1 \
        libmount1=2.33.1-0.1+deb10u1 \
        libsmartcols1=2.33.1-0.1+deb10u1 \
        libuuid1=2.33.1-0.1+deb10u1 \
        mount=2.33.1-0.1+deb10u1 \
        util-linux=2.33.1-0.1+deb10u1 \
        libgssapi-krb5-2=1.17-3+deb10u6 \
        libk5crypto3=1.17-3+deb10u6 \
        libkrb5-3=1.17-3+deb10u6 \
        libkrb5support0=1.17-3+deb10u6 \
        tar=1.30+dfsg-6+deb10u1 \
        tzdata=2024a-0+deb10u1 \
        debian-archive-keyring=2019.1+deb10u2 \
    && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["dotnet", "BilibiliMangaAutoClockIn.dll"]
