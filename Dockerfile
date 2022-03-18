FROM golang:1.17.8-bullseye AS gobuild

# install dependencies
RUN apt-get update \
    && apt-get install -y --no-install-recommends g++ make gcc git build-essential ca-certificates curl \
    && update-ca-certificates

ENV GO111MODULE=on
WORKDIR /goapp

COPY go.mod .
COPY go.sum .
RUN go mod download

# static build
ADD . .
RUN go build -a --ldflags '-extldflags "-static"' main.go

# copy executable file and certs to a pure container
FROM debian:11.2

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates haveged \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=gobuild /goapp/main /app/go-ramjet
COPY --from=gobuild /etc/ssl/certs /etc/ssl/certs
COPY --from=gobuild /go/pkg/mod/github.com/yanyiwu/gojieba@v1.0.0 /go/pkg/mod/github.com/yanyiwu/gojieba@v1.0.0

ENTRYPOINT ["/app/go-ramjet"]
