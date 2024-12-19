# JaegerAI: Using GenAI for user-friendly Microservices Observability



We employ a novel Graph Retrieval Augmented Generation (RAG) technique that summarize microservice logs  and retain hierarchical relationship of API invocations. We developed a custom storage  plugin that runs alongside Jaeger. Our method is able to achieve insert performance outperforming naive RAG by insert performance.


### Modules

#### jaeger-storage

Neo4j, GPT, and Graph RAG is here. It is exposed as a gRPC server. Jaeger backend will forward its spans here.

#### jaeger-ui

A clone of Jaeger UI project. We extended the UI to allow a Q&A interface to assist developers in debugging their microservice faults.

#### eval

Evaluation scripts done here,

#### HotrodData

Scrap experiments exporting JSON from Jaeger and summarizing the logs.

#### jaeger-1.62.0-linux-amd64

Jaeger binary to run in Ubuntu.

### How to get this running?

1. Add your OpenAI API key as `OPENAI_API_KEY` env variable.
2. Run `jaeger-storage/run.sh`.
3. Open Neo4j browser in http://localhost:7474/browser/, execute the `jaeger-storage/migrations/constraint.cql`. (one time operation)
4. Execute

```shell
SPAN_STORAGE_TYPE=grpc ./jaeger-1.62.0-linux-amd64/jaeger-collector --grpc-storage.server=localhost:54321 \
& OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 ./jaeger-1.62.0-linux-amd64/example-hotrod all
```

5. Run `npm start` in `jaeger-ui`.
6. Browse to http://localhost:5173 to open Jaeger UI dashboard.
