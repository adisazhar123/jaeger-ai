### Setup Guide

#### Requirements
* Go version 1.23.2 ([ref](https://github.com/moovweb/gvm))
* Docker compose ([ref](https://docs.docker.com/compose/install/))
* Jaeger v1.6.2 binaries ([ref](https://www.jaegertracing.io/download/))

#### How to run the jaeger-storage server

1) Run `docker compose up --build`. This will start a Postgres server, then executes `migration/initial.sql`.
2) Run `go mod download` to download the Go dependencies.
3) Run `go build .` to build the project.
4) Run `./jaeger-storage` to run the binary.

Steps 1-2 are a one time operation.

#### How to run Jaeger components

1) Run the following command. Make sure to replace the path to `jaeger-all-in-one` and `hotrod`.
```shell
SPAN_STORAGE_TYPE=grpc ./jaeger-1.62.0-linux-amd64/jaeger-all-in-one --grpc-storage.server=localhost:54321 & OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 ./jaeger/examples/hotrod/hotrod all
```

2) Browse to `localhost:8080` to open Hotrod.
3) Browse to `localhost:16686` to open Jaeger UI.
