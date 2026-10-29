FROM mcr.microsoft.com/dotnet/sdk:6.0 AS build-env
WORKDIR /app

COPY src/. ./
RUN dotnet publish Mix.Cms.Web/Mixcore.csproj -c Release

# Build runtime image
# Using aspnet:6.0 instead of sdk:6.0 to reduce attack surface and image size.
# The base image tag is kept as 6.0 to match the target framework, but in production,
# a specific patched version should be used if available in the registry.
# Note: The vulnerability scan indicates many OS-level vulnerabilities (libperl, curl, openssl, etc.)
# which are fixed by updating the base OS packages. Since we cannot easily update the entire OS
# without breaking the .NET runtime dependencies in the official image, and the summary suggests
# "Prefer a single base image upgrade", moving from SDK to ASP.NET Runtime is the primary optimization
# to remove build tools (git, perl, etc.) which are a large source of vulnerabilities.
#
# Additionally, we will attempt to update the remaining OS packages in the runtime stage
# to address the specific OS vulnerabilities listed (perl, curl, openssl, etc.) before copying the app.
FROM mcr.microsoft.com/dotnet/aspnet:6.0 AS runtime
WORKDIR /app

# Update OS packages to fix vulnerabilities.
# This step is crucial to address the 660 findings with fixes available in OS-level packages.
# We run apt-get update and upgrade to get the latest security patches for the Debian 11 base.
RUN apt-get update && \
    apt-get upgrade -y --no-install-recommends && \
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
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Create a non-root user
RUN groupadd --system --gid 1001 appuser && \
    useradd --system --uid 1001 --gid 1001 --home-dir /app appuser

# Copy application from build stage
COPY --from=build-env --chown=appuser:appuser /app/Mix.Cms.Web/bin/Release/net6.0/publish .

ENV ASPNETCORE_URLS=http://+:80
EXPOSE 80

# Switch to non-root user
USER appuser

ENTRYPOINT ["dotnet", "mixcore.dll"]
