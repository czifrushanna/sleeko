FROM mcr.microsoft.com/dotnet/aspnet:8.0.32@sha256:2e0a5a1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f AS base
WORKDIR /app
EXPOSE 8080

FROM mcr.microsoft.com/dotnet/sdk:8.0.400@sha256:2e0a5a1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f2c1f AS build
WORKDIR /code
COPY ["src/Ray.BiliBiliTool.Web/Ray.BiliBiliTool.Web.csproj", "src/Ray.BiliBiliTool.Web/"]
COPY ["src/Ray.BiliBiliTool.Web.Client/Ray.BiliBiliTool.Web.Client.csproj", "src/Ray.BiliBiliTool.Web.Client/"]
COPY ["src/Ray.BiliBiliTool.Application/Ray.BiliBiliTool.Application.csproj", "src/Ray.BiliBiliTool.Application/"]
COPY ["src/Ray.BiliBiliTool.Application.Contracts/Ray.BiliBiliTool.Application.Contracts.csproj", "src/Ray.BiliBiliTool.Application.Contracts/"]
COPY ["src/Ray.BiliBiliTool.Domain/Ray.BiliBiliTool.Domain.csproj", "src/Ray.BiliBiliTool.Domain/"]
COPY ["src/Ray.BiliBiliTool.DomainService/Ray.BiliBiliTool.DomainService.csproj", "src/Ray.BiliBiliTool.DomainService/"]
COPY ["src/Ray.BiliBiliTool.Config/Ray.BiliBiliTool.Config.csproj", "src/Ray.BiliBiliTool.Config/"]
COPY ["src/Ray.BiliBiliTool.Agent/Ray.BiliBiliTool.Agent.csproj", "src/Ray.BiliBiliTool.Agent/"]
COPY ["src/Ray.BiliBiliTool.Infrastructure/Ray.BiliBiliTool.Infrastructure.csproj", "src/Ray.BiliBiliTool.Infrastructure/"]
COPY ["src/Ray.BiliBiliTool.Infrastructure.EF/Ray.BiliBiliTool.Infrastructure.EF.csproj", "src/Ray.BiliBiliTool.Infrastructure.EF/"]
COPY ["src/BlazingQuartz.Core/BlazingQuartz.Core.csproj", "src/BlazingQuartz.Core/"]
COPY ["src/BlazingQuartz.Jobs/BlazingQuartz.Jobs.csproj", "src/BlazingQuartz.Jobs/"]
COPY ["src/BlazingQuartz.Jobs.Abstractions/BlazingQuartz.Jobs.Abstractions.csproj", "src/BlazingQuartz.Jobs.Abstractions/"]
RUN dotnet restore "src/Ray.BiliBiliTool.Web/Ray.BiliBiliTool.Web.csproj"
COPY . .
WORKDIR "/code/src/Ray.BiliBiliTool.Web"
RUN dotnet build "Ray.BiliBiliTool.Web.csproj" -c Release -o /app/build

FROM build AS publish
RUN dotnet publish "Ray.BiliBiliTool.Web.csproj" -c Release -o /app/publish

FROM base AS final
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends perl-base=5.36.0-7+deb12u4 && rm -rf /var/lib/apt/lists/*
COPY --from=publish /app/publish .
ENTRYPOINT ["dotnet", "Ray.BiliBiliTool.Web.dll"]
