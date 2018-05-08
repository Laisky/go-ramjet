# GO-Ramjet

## Dockerlize

Make docker image

```sh
docker build . -t ppcelery/go-ramjet:pateo
docker push ppcelery/go-ramjet:pateo
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
    ppcelery/go-ramjet:pateo
```
