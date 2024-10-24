CREATE TYPE SPANKIND AS ENUM ('server', 'client', 'unspecified', 'producer', 'consumer', 'ephemeral', 'internal');

CREATE TABLE services
(
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE operations
(
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT                            NOT NULL,
    service_id BIGINT REFERENCES services (id) NOT NULL,
    kind       SPANKIND                        NOT NULL,

    UNIQUE (name, kind, service_id)
);

CREATE TABLE spans
(
    id      BIGSERIAL PRIMARY KEY,
    span_id      TEXT                             NOT NULL,
    trace_id     TEXT                             NOT NULL,
    operation_id BIGINT REFERENCES operations (id) NOT NULL,
    flags        BIGINT                            NOT NULL,
    start_time   TIMESTAMP                         NOT NULL,
    duration     INTERVAL                          NOT NULL,
    tags         JSONB,
    service_id   BIGINT REFERENCES services (id)   NOT NULL,
    process_id   TEXT                              NOT NULL,
    process_tags JSONB                             NOT NULL,
    warnings     TEXT[],
    logs         JSONB,
    kind         SPANKIND                          NOT NULL,
    refs         JSONB                             NOT NULL
);

DROP TABLE spans;
DROP TABLE operations;
DROP TABLE services;

DROP TYPE SPANKIND;
