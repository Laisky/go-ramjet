FROM golang:1.21.1-bullseye AS gobuild

# install dependencies
RUN apt-get update && apt-get install -y --no-install-recommends g++ make gcc git \
    build-essential ca-certificates curl \
    && update-ca-certificates

ENV GO111MODULE=on
WORKDIR /goapp

COPY go.mod .
COPY go.sum .
RUN go mod download

# static build
ADD . .
ENV GOOS=linux
ENV GOARCH=amd64
RUN go build -a --ldflags '-extldflags "-static"' main.go

# copy executable file and certs to a pure container
FROM debian:bullseye

RUN apt-get update
RUN apt-get install -y --no-install-recommends ca-certificates haveged \
    # for google-chrome
    # libappindicator1 fonts-liberation xdg-utils wget \
    # libasound2 libatk-bridge2.0-0 libatspi2.0-0 libcurl3-gnutls libcurl3-nss \
    # libcurl4 libcurl3 libdrm2 libgbm1 libgtk-3-0 libgtk-4-1 libnspr4 libnss3 \
    # libu2f-udev libvulkan1 libxkbcommon0 \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# install google-chrome
ENV PATH=/usr/local/bin:$PATH
RUN wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb \
    && apt install google-chrome-stable_current_amd64.deb \
    && rm google-chrome-stable_current_amd64.deb

COPY --from=gobuild /goapp/main /app/go-ramjet
COPY --from=gobuild /etc/ssl/certs /etc/ssl/certs
COPY --from=gobuild /go/pkg/mod/github.com/yanyiwu/gojieba@v1.3.0 /go/pkg/mod/github.com/yanyiwu/gojieba@v1.3.0

ENTRYPOINT ["/app/go-ramjet"]
