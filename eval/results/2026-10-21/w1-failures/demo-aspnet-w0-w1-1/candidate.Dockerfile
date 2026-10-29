FROM mcr.microsoft.com/dotnet/aspnet:5.0 AS base
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends \
    libc-bin=2.28-10+deb10u4 \
    libc6=2.28-10+deb10u4 \
    zlib1g=1:1.2.11.dfsg-1+deb10u2 \
    libtasn1-6=4.13-3+deb10u1 \
    libssl1.1=1.1.1n-0+deb10u6 \
    openssl=1.1.1n-0+deb10u6 \
    libsystemd0=241-7~deb10u10 \
    libudev1=241-7~deb10u10 \
    libgnutls30=3.6.7-4+deb10u12 \
    libncursesw6=6.1+20181013-2+deb10u5 \
    libtinfo6=6.1+20181013-2+deb10u5 \
    ncurses-base=6.1+20181013-2+deb10u5 \
    ncurses-bin=6.1+20181013-2+deb10u5 \
    libgssapi-krb5-2=1.17-3+deb10u6 \
    libk5crypto3=1.17-3+deb10u6 \
    libkrb5-3=1.17-3+deb10u6 \
    libkrb5support0=1.17-3+deb10u6 \
    bsdutils=1:2.33.1-0.1+deb10u1 \
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

FROM mcr.microsoft.com/dotnet/sdk:5.0 AS build
COPY . /src
WORKDIR /src
RUN dotnet build "aspnetapp.csproj" -c Release -o /app/build

FROM build AS publish
RUN dotnet publish "aspnetapp.csproj" -c Release -o /app/publish

FROM base AS final
WORKDIR /app
COPY --from=publish /app/publish .
ENTRYPOINT ["dotnet", "aspnetapp.dll"]
