# db . -t ppcelery/go-ramjet:latest
FROM node:20 AS nodebuild

RUN npm install -g sass
WORKDIR /app
ADD . .
RUN sass ./internal/tasks/gptchat/templates/scss

# =====================================

FROM golang:1.23.5-bullseye AS gobuild

# install dependencies
RUN apt-get update && apt-get install -y --no-install-recommends g++ make gcc git \
    build-essential ca-certificates curl \
    && update-ca-certificates

# install azure sdk
# https://learn.microsoft.com/en-us/azure/ai-services/speech-service/quickstarts/setup-platform?tabs=windows%2Cubuntu%2Cdotnetcli%2Cdotnet%2Cjre%2Cmaven%2Cbrowser%2Cmac%2Cpypi&pivots=programming-language-go
RUN apt-get install -y libssl-dev ca-certificates libasound2 wget
ENV SPEECHSDK_ROOT=/opt/azure/speech
ENV CGO_CFLAGS="-I$SPEECHSDK_ROOT/include/c_api"
ENV CGO_LDFLAGS="-L$SPEECHSDK_ROOT/lib/x64 -lMicrosoft.CognitiveServices.Speech.core"
ENV LD_LIBRARY_PATH="$SPEECHSDK_ROOT/lib/x64:$LD_LIBRARY_PATH"
RUN mkdir -p $SPEECHSDK_ROOT
RUN wget -O SpeechSDK-Linux.tar.gz https://s3.laisky.com/public/SpeechSDK-Linux.tar.gz \
    && tar --strip 1 -xzf SpeechSDK-Linux.tar.gz -C "$SPEECHSDK_ROOT"

ENV GO111MODULE=on
WORKDIR /goapp

COPY go.mod .
COPY go.sum .
RUN go mod download

# static build
ADD . .
COPY --from=nodebuild /app/internal/tasks/gptchat/templates/scss/*.css ./internal/tasks/gptchat/templates/scss/.
ENV GOOS=linux
ENV GOARCH=amd64
RUN go build

# =====================================

# copy executable file and certs to a pure container
FROM debian:bullseye

RUN apt-get update
RUN apt-get install -y --no-install-recommends ca-certificates haveged wget \
    # for google-chrome
    # libappindicator1 fonts-liberation xdg-utils wget \
    # libasound2 libatk-bridge2.0-0 libatspi2.0-0 libcurl3-gnutls libcurl3-nss \
    # libcurl4 libcurl3 libdrm2 libgbm1 libgtk-3-0 libgtk-4-1 libnspr4 libnss3 \
    # libu2f-udev libvulkan1 libxkbcommon0 \
    && update-ca-certificates 2>/dev/null || true

# install google-chrome
# RUN wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb \
RUN wget https://s3.laisky.com/public/google-chrome-stable_current_amd64.deb \
    && apt install -y ./google-chrome-stable_current_amd64.deb \
    && rm google-chrome-stable_current_amd64.deb

# install azure sdk
RUN apt-get install -y libssl-dev ca-certificates libasound2 wget
ENV SPEECHSDK_ROOT=/opt/azure/speech
ENV CGO_CFLAGS="-I$SPEECHSDK_ROOT/include/c_api"
ENV CGO_LDFLAGS="-L$SPEECHSDK_ROOT/lib/x64 -lMicrosoft.CognitiveServices.Speech.core"
ENV LD_LIBRARY_PATH="$SPEECHSDK_ROOT/lib/x64:$LD_LIBRARY_PATH"
RUN mkdir -p $SPEECHSDK_ROOT
COPY --from=gobuild /opt/azure/speech $SPEECHSDK_ROOT

# apt finished, clean cache
RUN rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=gobuild /etc/ssl/certs /etc/ssl/certs
COPY --from=gobuild /go/pkg/mod/github.com/yanyiwu/gojieba@v1.4.4 /go/pkg/mod/github.com/yanyiwu/gojieba@v1.4.4
COPY --from=gobuild /goapp/go-ramjet /app/go-ramjet

ENTRYPOINT ["/app/go-ramjet"]
