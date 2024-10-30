package main

import (
	"context"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"log"
	"time"
)

type WriterDbClient struct {
	db *sqlx.DB
}

func NewWriterDBClient(db *sqlx.DB) *WriterDbClient {
	return &WriterDbClient{db: db}
}

func (c *WriterDbClient) upsertService(ctx context.Context, p InternalService) (int64, error) {
	//goland:noinspection ALL
	query := "WITH new_services AS (INSERT INTO services(name, created_at) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING RETURNING id) SELECT COALESCE((SELECT id FROM new_services),(SELECT id FROM services WHERE name = $3 AND deleted_at IS NULL )) as id"
	fmt.Println(fmt.Sprintf("[upsertService] service name: %s", p.Name))

	var id int64

	err := c.db.GetContext(ctx, &id, query, p.Name, p.CreatedAt, p.Name)
	if err != nil {
		fmt.Println("[upsertService] err", err)
		return 0, err
	}

	fmt.Println(fmt.Sprintf("returned service id %d", id))

	return id, nil
}

func (c *WriterDbClient) insertOperation(ctx context.Context, p InternalOperation) (int64, error) {
	//goland:noinspection ALL
	query := "WITH new_operation AS (INSERT INTO operations(name, service_id, kind, created_at) values ($1, $2, $3, $4) ON CONFLICT(name, kind, service_id) DO NOTHING RETURNING id) SELECT COALESCE((SELECT id FROM new_operation), (SELECT id from operations WHERE name = $5 AND kind = $6 AND service_id = $7 AND deleted_at IS NULL)) as id"

	var id int64

	err := c.db.GetContext(ctx, &id, query, p.Name, p.ServiceId, p.Kind, p.CreatedAt, p.Name, p.Kind, p.ServiceId)

	return id, err
}

func (c *WriterDbClient) insertSpan(ctx context.Context, p InternalSpan) (int64, error) {
	//goland:noinspection ALL
	query := "INSERT INTO spans(span_id, trace_id, operation_id, flags, start_time, duration, tags, service_id, process_id, process_tags, warnings, logs, kind, refs, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING id"
	var id int64
	err := c.db.GetContext(ctx, &id, query, p.SpanId, p.TraceId, p.OperationId, p.Flags, p.StartTime, p.Duration.Seconds(), p.Tags, p.ServiceId, p.ProcessId, p.ProcessTags, p.WarningsPq, p.Logs, p.Kind, p.Refs, p.CreatedAt)

	return id, err
}

func (c *WriterDbClient) WriteSpan(ctx context.Context, span *model.Span) error {
	log.Println(fmt.Sprintf("[writespan] received a request to write a span, spanId: %s, serviceName: %s, operationName: %s", span.SpanID.String(), span.Process.GetServiceName(), span.GetOperationName()))

	//	upsert InternalService
	serviceId, err := c.upsertService(ctx, InternalService{
		Name:      span.Process.GetServiceName(),
		CreatedAt: time.Now(),
	})
	if err != nil {
		log.Println("[writespan][error] cannot upsert InternalService", err)
		return err
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
		Flags:       uint64(span.Flags),
		StartTime:   span.StartTime,
		Duration:    span.Duration,
		Tags:        tags,
		ServiceId:   serviceId,
		ProcessId:   span.ProcessID,
		ProcessTags: processTags,
		Warnings:    span.Warnings,
		WarningsPq:  pq.Array(span.Warnings),
		Logs:        logs,
		Kind:        spanKind.String(),
		Refs:        references,
		CreatedAt:   time.Now(),
	}
	spanId, err := c.insertSpan(ctx, spanData)
	if err != nil {
		log.Println("[writespan][error] an error occurred while inserting span", err)
		log.Println(fmt.Sprintf("span data %+v", spanData))
		return err
	}
	log.Println(fmt.Sprintf("[writespan] successfully inserted span with primary key: %d, spanId: %s, serviceName: %s, operationName: %s", spanId, span.SpanID.String(), span.Process.GetServiceName(), span.GetOperationName()))

	return nil
}
