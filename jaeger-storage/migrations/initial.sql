-- adapted from https://github.com/robbert229/jaeger-postgresql/blob/main/internal/sql/migrations/001_initial.sql
-- with minor adjustments towards the column type and additional created_at & deleted_at columns

BEGIN
TRANSACTION;

DROP TABLE IF EXISTS traces, operations, services, spans;
DROP TYPE IF EXISTS SPANKIND;

CREATE TYPE SPANKIND AS ENUM ('server', 'client', 'unspecified', 'producer', 'consumer', 'ephemeral', 'internal');


CREATE TABLE IF NOT EXISTS traces
(
    id         BIGSERIAL PRIMARY KEY,
    trace_id   TEXT        NOT NULL,
    summary    TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);


CREATE TABLE IF NOT EXISTS services
(
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS operations
(
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT                            NOT NULL,
    service_id BIGINT REFERENCES services (id) NOT NULL,
    kind       SPANKIND                        NOT NULL,
    created_at TIMESTAMPTZ                     NOT NULL,
    deleted_at TIMESTAMPTZ,

    UNIQUE (name, kind, service_id)
    );

CREATE TABLE IF NOT EXISTS spans
(
    id           BIGSERIAL PRIMARY KEY,
    span_id      TEXT                              NOT NULL,
    trace_id_ref BIGINT REFERENCES traces (id),
    trace_id     TEXT                              NOT NULL,
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
    refs         JSONB                             NOT NULL,
    summary      TEXT,
    created_at   TIMESTAMPTZ                       NOT NULL,
    deleted_at   TIMESTAMPTZ
    );

END
TRANSACTION;
