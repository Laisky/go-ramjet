FROM golang:1.13.4-alpine3.10 AS gobuild

# run dependencies
RUN apk update && apk upgrade && \
    apk add --no-cache gcc build-base git ca-certificates && \
    update-ca-certificates

ENV GO111MODULE=on
WORKDIR /goapp

COPY go.mod .
COPY go.sum .
RUN go mod download

# static build
ADD . .
RUN go build -a --ldflags '-extldflags "-static"' entrypoints/main.go

# copy executable file and certs to a pure container
FROM alpine:3.13.6
COPY --from=gobuild /goapp/main go-ramjet
COPY --from=gobuild /etc/ssl/certs /etc/ssl/certs
COPY --from=gobuild /go/pkg/mod/github.com/yanyiwu/gojieba@v1.0.0 /go/pkg/mod/github.com/yanyiwu/gojieba@v1.0.0

CMD ["./go-ramjet", "--config=/etc/go-ramjet/settings"]
