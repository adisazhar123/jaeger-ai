package main

import (
	"context"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"log"
	"time"
)

type WriterDBClient struct {
	db *sqlx.DB
}

type WriterOpt struct {
	Username string
	Password string
	DbName   string
}

func NewWriterDBClient(db *sqlx.DB) *WriterDBClient {
	return &WriterDBClient{db: db}
}

type service struct {
	Id        int64      `db:"id"`
	Name      string     `db:"name"`
	CreatedAt time.Time  `db:"created_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}

func (c WriterDBClient) upsertService(ctx context.Context, p service) (int64, error) {
	//goland:noinspection ALL
	query := "WITH new_services AS (INSERT INTO services(name, created_at) VALUES (:name, :created_at) ON CONFLICT (name) DO NOTHING RETURNING id) SELECT COALESCE((SELECT id FROM new_services),(SELECT id FROM services WHERE name = :name AND deleted_at IS NULL )) as id"

	rows, err := c.db.NamedQueryContext(ctx, query, p)
	if err != nil {
		return 0, err
	}

	res := struct {
		Id int64 `db:"id"`
	}{}

	for rows.Next() {
		if err := rows.StructScan(&res); err != nil {
			return 0, err
		}
	}

	return res.Id, nil
}

func (c WriterDBClient) insertOperation(ctx context.Context, p InternalOperation) (int64, error) {
	//goland:noinspection ALL
	query := "WITH new_operation AS (INSERT INTO operations(name, service_id, kind, created_at) values (:name, :service_id, :kind, :created_at) ON CONFLICT(name, kind, service_id) DO NOTHING RETURNING id) SELECT COALESCE((SELECT id FROM new_operation), (SELECT id from operations WHERE name = :name AND kind = :kind AND service_id = :service_id AND deleted_at IS NULL)) as id"

	rows, err := c.db.NamedQueryContext(ctx, query, p)
	if err != nil {
		return 0, err
	}

	res := struct {
		Id int64 `db:"id"`
	}{}

	for rows.Next() {
		if err := rows.StructScan(&res); err != nil {
			return 0, err
		}
	}

	return res.Id, nil
}

func (c WriterDBClient) insertSpan(ctx context.Context, p InternalSpan) (int64, error) {
	//goland:noinspection ALL
	query := "INSERT INTO spans(span_id, trace_id, operation_id, flags, start_time, duration, tags, service_id, process_id, process_tags, warnings, logs, kind, refs) VALUES (:span_id, :trace_id, :operation_id, :flags, :start_time, :duration, :tags, :service_id, :process_id, :process_tags, :warnings, :logs, :kind, :refs) RETURNING *"

	result, err := c.db.NamedExecContext(ctx, query, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (c WriterDBClient) WriteSpan(ctx context.Context, span *model.Span) error {
	log.Println(fmt.Sprintf("[writespan] received a request to write a span, spanId: %s, serviceName: %s, operationName: %s", span.SpanID.String(), span.Process.GetServiceName(), span.GetOperationName()))

	//	upsert service
	serviceId, err := c.upsertService(ctx, service{
		Name:      span.Process.GetServiceName(),
		CreatedAt: time.Now(),
	})
	if err != nil {
		log.Println("[writespan][error] cannot upsert service", err)
	}
	spanKind, _ := span.GetSpanKind()
	//	upsert operation
	operationId, err := c.insertOperation(ctx, InternalOperation{
		Name:      span.GetOperationName(),
		ServiceId: serviceId,
		Kind:      spanKind.String(),
		CreatedAt: time.Now(),
	})

	tags, err := encodeTags(span.Tags)
	if err != nil {
		log.Println("[writespan][error] an error occurred while encoding tags", err)
		return err
	}
	processTags, err := encodeTags(span.Process.Tags)
	if err != nil {
		log.Println("[writespan][error] an error occurred while encoding process tags", err)
		return err
	}

	logs, err := encodeLogs(span.Logs)
	if err != nil {
		log.Println("[writespan][error] an error occurred while encoding logs", err)
		return err
	}

	references, err := encodeReferences(span.References)
	if err != nil {
		log.Println("[writespan][error] an error occurred while encoding references", err)
		return err
	}

	//	insert span
	spanData := InternalSpan{
		SpanId:      span.SpanID.String(),
		TraceId:     span.TraceID.String(),
		OperationId: operationId,
		Flags:       int64(span.Flags),
		StartTime:   span.StartTime,
		Duration:    span.Duration,
		Tags:        tags,
		ServiceId:   serviceId,
		ProcessId:   span.ProcessID,
		ProcessTags: processTags,
		Warnings:    span.Warnings,
		Logs:        logs,
		Kind:        spanKind.String(),
		Refs:        references,
		CreatedAt:   time.Now(),
	}
	spanId, err := c.insertSpan(ctx, spanData)
	if err != nil {
		log.Println("[writespan][error] an error occurred while inserting span", err)
		log.Println("span data", spanData)
		return err
	}
	log.Println(fmt.Sprintf("[writespan] successfully inserted span with id %d", spanId))

	return nil
}
