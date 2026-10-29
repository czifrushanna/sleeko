FROM mcr.microsoft.com/dotnet/sdk:6.0 AS build-env
WORKDIR /app

COPY src/. ./
RUN dotnet publish Mix.Cms.Web/Mixcore.csproj -c Release

# Build runtime image
FROM mcr.microsoft.com/dotnet/aspnet:6.0 AS runtime
WORKDIR /app

# Address system package vulnerabilities identified in the scan summary
# These packages are part of the base image and can be updated via apt-get
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    libperl5.32=5.32.1-4+deb11u5 \
    perl=5.32.1-4+deb11u5 \
    perl-base=5.32.1-4+deb11u5 \
    perl-modules-5.32=5.32.1-4+deb11u5 \
    libgnutls30=3.7.1-5+deb11u10 \
    curl=7.74.0-1.3+deb11u16 \
    libcurl3-gnutls=7.74.0-1.3+deb11u16 \
    libcurl4=7.74.0-1.3+deb11u16 \
    wget=1.21-1+deb11u2 \
    libexpat1=2.2.10-2+deb11u7 \
    libssl1.1=1.1.1w-0+deb11u8 \
    openssl=1.1.1w-0+deb11u8 \
    libgssapi-krb5-2=1.18.3-6+deb11u8 \
    libk5crypto3=1.18.3-6+deb11u8 \
    libkrb5-3=1.18.3-6+deb11u8 \
    libkrb5support0=1.18.3-6+deb11u8 \
    git=1:2.30.2-1+deb11u5 \
    git-man=1:2.30.2-1+deb11u5 \
    libsystemd0=247.3-7+deb11u8 \
    libudev1=247.3-7+deb11u8 \
    libpam-modules=1.4.0-9+deb11u2 \
    libpam-modules-bin=1.4.0-9+deb11u2 \
    libpam-runtime=1.4.0-9+deb11u2 \
    libpam0g=1.4.0-9+deb11u2 \
    gpgv=2.2.27-2+deb11u3 \
    libnghttp2-14=1.43.0-1+deb11u3 \
    libc-bin=2.31-13+deb11u14 \
    libc6=2.31-13+deb11u14 \
    login=1:4.8.1-1+deb11u1 \
    passwd=1:4.8.1-1+deb11u1 \
    libtasn1-6=4.16.0-2+deb11u2 \
    liblzma5=5.2.5-2.1~deb11u2 \
    libicu67=67.1-7+deb11u1 \
    dpkg=1.20.14 \
    tzdata=2026b-0+deb11u1 \
    ca-certificates=20250419~deb12u1~deb11u1 && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build-env /app/Mix.Cms.Web/bin/Release/net6.0/publish .

ENV ASPNETCORE_URLS=http://+:80
EXPOSE 80

ENTRYPOINT ["dotnet", "mixcore.dll"]
