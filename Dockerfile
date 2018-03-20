FROM golang:1.9.4-alpine3.6 AS gobin
RUN mkdir -p /go/src/github.com/go-ramjet
RUN apk update && apk upgrade && \
    apk add --no-cache bash git openssh
ADD . /go/src/github.com/go-ramjet
WORKDIR /go/src/github.com/go-ramjet
RUN go build main.go

FROM alpine:3.6
COPY --from=gobin /go/src/github.com/go-ramjet/main .
CMD ["./main"]
