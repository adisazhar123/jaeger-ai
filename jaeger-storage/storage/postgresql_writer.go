package storage

import (
	"context"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"jaeger-storage/common"
	"log"
	"time"
)

type SqlWriter struct {
	db *sqlx.DB
}

func NewSqlWriter(db *sqlx.DB) *SqlWriter {
	return &SqlWriter{db: db}
}

func (w *SqlWriter) upsertService(ctx context.Context, p common.InternalService) (int64, error) {
	//goland:noinspection ALL
	query := "WITH new_services AS (INSERT INTO services(name, created_at) VALUES ($1, $2) ON CONFLICT (name) DO NOTHING RETURNING id) SELECT COALESCE((SELECT id FROM new_services),(SELECT id FROM services WHERE name = $3 AND deleted_at IS NULL )) as id"
	fmt.Println(fmt.Sprintf("[sql][upsertService] service name: %s", p.Name))

	var id int64

	err := w.db.GetContext(ctx, &id, query, p.Name, p.CreatedAt, p.Name)
	if err != nil {
		fmt.Println("[sql][upsertService] err", err)
		return 0, err
	}

	fmt.Printf("[sql][upsertService] returned service id %d\n", id)

	return id, nil
}

func (w *SqlWriter) upsertOperation(ctx context.Context, p common.InternalOperation) (int64, error) {
	//goland:noinspection ALL
	query := "WITH new_operation AS (INSERT INTO operations(name, service_id, kind, created_at) values ($1, $2, $3, $4) ON CONFLICT(name, kind, service_id) DO NOTHING RETURNING id) SELECT COALESCE((SELECT id FROM new_operation), (SELECT id from operations WHERE name = $5 AND kind = $6 AND service_id = $7 AND deleted_at IS NULL)) as id"

	var id int64

	err := w.db.GetContext(ctx, &id, query, p.Name, p.ServiceId, p.Kind, p.CreatedAt, p.Name, p.Kind, p.ServiceId)

	return id, err
}

func (w *SqlWriter) insertSpan(ctx context.Context, p common.InternalSpan) (int64, error) {
	//goland:noinspection ALL
	query := "INSERT INTO spans(span_id, trace_id, operation_id, flags, start_time, duration, tags, service_id, process_id, process_tags, warnings, logs, kind, refs, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING id"
	var id int64
	err := w.db.GetContext(ctx, &id, query, p.SpanId, p.TraceId, p.OperationId, p.Flags, p.StartTime, p.Duration.Seconds(), p.Tags, p.ServiceId, p.ProcessId, p.ProcessTags, p.WarningsPq, p.Logs, p.Kind, p.Refs, p.CreatedAt)

	return id, err
}

func (w *SqlWriter) WriteSpan(ctx context.Context, span *model.Span, tags, processTags, logs, references []byte) error {
	//	upsert InternalService
	serviceId, err := w.upsertService(ctx, common.InternalService{
		Name:      span.Process.GetServiceName(),
		CreatedAt: time.Now(),
	})
	if err != nil {
		log.Println("[sql][writespan][error] cannot upsert InternalService", err)
		return err
	}
	spanKind, _ := span.GetSpanKind()
	//	upsert operation
	operationId, err := w.upsertOperation(ctx, common.InternalOperation{
		Name:      span.GetOperationName(),
		ServiceId: serviceId,
		Kind:      spanKind.String(),
		CreatedAt: time.Now(),
	})

	//	insert span
	spanData := common.InternalSpan{
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
	spanId, err := w.insertSpan(ctx, spanData)
	if err != nil {
		log.Println("[sql][writespan][error] an error occurred while inserting span", err)
		log.Printf("[sql][writespan] span data %+v", spanData)
		return err
	}
	log.Println(fmt.Sprintf("[sql][writespan] successfully inserted span with primary key: %d, spanId: %s, serviceName: %s, operationName: %s", spanId, span.SpanID.String(), span.Process.GetServiceName(), span.GetOperationName()))
	return nil
}
