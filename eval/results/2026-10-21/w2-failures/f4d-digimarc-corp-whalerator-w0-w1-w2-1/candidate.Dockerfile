FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build
WORKDIR /src
COPY lib .
WORKDIR /src/Whalerator.WebAPI
RUN dotnet build -c Release -o /app

FROM build AS publish
RUN dotnet publish -c Release -o /app

FROM node:fermium AS ngbuild
COPY web/src /web/src
COPY web/package.json /web/
COPY web/package-lock.json /web/
COPY web/angular.json /web/
COPY web/tsconfig.json /web/
WORKDIR /web
RUN npm install && npm install @angular/cli && /web/node_modules/@angular/cli/bin/ng.js build --configuration production --output-path /dist

FROM mcr.microsoft.com/dotnet/aspnet:8.0 AS final
WORKDIR /app
COPY --from=publish /app .
COPY --from=ngbuild /dist ./wwwroot
COPY lib/Whalerator.WebAPI/config-docker.yaml config.yaml
ARG SRC_HASH="Unknown"
ARG RELEASE="0.0"
RUN echo "{ \""hash\"": \""$SRC_HASH\"", \""release\"": \""$RELEASE\"" }" > ./wwwroot/assets/v.json
COPY *.md /
EXPOSE 80
ENTRYPOINT ["dotnet", "Whalerator.WebAPI.dll"]
