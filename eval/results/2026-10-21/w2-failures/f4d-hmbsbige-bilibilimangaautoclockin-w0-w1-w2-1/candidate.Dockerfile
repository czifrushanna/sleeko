#See https://aka.ms/containerfastmode to understand how Visual Studio uses this Dockerfile to build your images for faster debugging.

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

# Update packages to fix vulnerabilities
# Note: We use `apt-get upgrade` to fetch the latest security patches available in the buster repositories.
# Pinning specific versions for every package is fragile and often results in version conflicts or packages not being found in the specific repo state.
# `apt-get upgrade` is the standard, robust way to apply security updates in Debian-based images.
# We run this in the final stage to ensure the runtime image is patched.
RUN apt-get update && \
    apt-get upgrade -y --no-install-recommends && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Create a non-root user for security best practices
RUN groupadd --system app && useradd --system --gid app --no-create-home app

# Copy published application
COPY --from=publish /app/publish .

# Set ownership to the non-root user
RUN chown -R app:app /app

# Switch to non-root user
USER app

ENTRYPOINT ["dotnet", "BilibiliMangaAutoClockIn.dll"]
