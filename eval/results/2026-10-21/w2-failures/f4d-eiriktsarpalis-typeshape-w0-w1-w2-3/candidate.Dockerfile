FROM mcr.microsoft.com/dotnet/sdk:7.0-bookworm-slim

ARG GIT_ASKPASS=/root/.git-askpass
ENV GIT_ASKPASS=$GIT_ASKPASS
ARG GIT_USER_NAME
ARG GIT_USER_EMAIL
RUN echo 'echo $GITHUB_TOKEN' > $GIT_ASKPASS && \
    chmod +x $GIT_ASKPASS && \
    git config --global user.name "$GIT_USER_NAME" && \
    git config --global user.email "$GIT_USER_EMAIL"

WORKDIR /app
COPY . .

ENV TARGET=Bundle
CMD ./build.sh -t $TARGET
