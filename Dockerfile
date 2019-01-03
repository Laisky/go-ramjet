FROM golang:1.11.4-alpine3.8 AS gobin

# http proxy
ENV HTTP_PROXY=http://172.16.4.26:17777
ENV HTTPS_PROXY=http://172.16.4.26:17777

# run dependencies
RUN apk update && apk upgrade && \
    apk add --no-cache g++ make gcc git ca-certificates && \
    update-ca-certificates
RUN mkdir -p /go/src/github.com/Laisky/go-ramjet
ADD . /go/src/github.com/Laisky/go-ramjet
WORKDIR /go/src/github.com/Laisky/go-ramjet

# static build
RUN go build --ldflags '-extldflags "-static"' entrypoints/main.go

# copy executable file and certs to a pure container
FROM alpine:3.8
COPY --from=gobin /go/src/github.com/Laisky/go-ramjet/main go-ramjet
COPY --from=gobin /etc/ssl/certs /etc/ssl/certs
COPY --from=gobin /go/src/github.com/Laisky/go-ramjet/vendor/github.com/yanyiwu/gojieba /go/src/github.com/Laisky/go-ramjet/vendor/github.com/yanyiwu/gojieba

CMD ["./go-ramjet", "--config=/etc/go-ramjet/settings"]
