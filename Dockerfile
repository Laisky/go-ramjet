FROM golang:1.10.1-alpine3.7 AS gobin
RUN apk update && apk upgrade && \
    apk add --no-cache g++ make gcc git ca-certificates && \
    update-ca-certificates
RUN mkdir -p /go/src/github.com/Laisky/go-ramjet
ADD . /go/src/github.com/Laisky/go-ramjet
WORKDIR /go/src/github.com/Laisky/go-ramjet
RUN go build --ldflags '-extldflags "-static"' entrypoints/main.go

FROM alpine:3.7
COPY --from=gobin /go/src/github.com/Laisky/go-ramjet/main go-ramjet
COPY --from=gobin /etc/ssl/certs /etc/ssl/certs
COPY --from=gobin /go/src/github.com/Laisky/go-ramjet/vendor/github.com/yanyiwu/gojieba /go/src/github.com/Laisky/go-ramjet/vendor/github.com/yanyiwu/gojieba

CMD ["./go-ramjet", "--config=/etc/go-ramjet/settings"]
