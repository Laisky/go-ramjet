# Go-Ramjet

[![Commitizen friendly](https://img.shields.io/badge/commitizen-friendly-brightgreen.svg)](http://commitizen.github.io/cz-cli/)
[![Go Report Card](https://goreportcard.com/badge/github.com/Laisky/go-ramjet)](https://goreportcard.com/report/github.com/Laisky/go-ramjet)
[![GoDoc](https://godoc.org/github.com/Laisky/go-ramjet?status.svg)](https://godoc.org/github.com/Laisky/go-ramjet)

Event-driven & Time-scheduler framwork.

## Web UI

The unified web UI is a React SPA located in [web/](web/). In production it is served by the Go server from `web/dist`.

Build both backend and frontend:

```sh
make build
```

Frontend-only (dev + tests):

```sh
make frontend-install
pnpm -C web dev
pnpm -C web test
```

## Dockerlize

Make docker image

```sh
docker build . -t ppcelery/go-ramjet:latest
docker push ppcelery/go-ramjet:latest
```

Run

```sh
# test
docker run -it --rm \
    -v /etc/go-ramjet/settings/settings.yml:/etc/go-ramjet/settings/settings.yml \
    -v /data/fluentd/fluentd-conf/backups:/data/fluentd/fluentd-conf/backups \
    -e TASKS=heartbeat \
    ppcelery/go-ramjet:test \
    /main --debug

# prod
docker run -it --rm
    -v /etc/go-ramjet/settings/settings.yml:/etc/go-ramjet/settings/settings.yml \
    -v /data/fluentd/fluentd-conf/backups:/data/fluentd/fluentd-conf/backups \
    ppcelery/go-ramjet:latest
```
