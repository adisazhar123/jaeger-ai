package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jmoiron/sqlx"
	"jaeger-storage/common"
	"log"
)

type ReaderDbClient struct {
	db *sqlx.DB
}

func NewReaderDBClient(db *sqlx.DB) *ReaderDbClient {
	return &ReaderDbClient{db: db}
}

func (r *ReaderDbClient) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	log.Println("in get trace", traceID.String())
	//goland:noinspection ALL
	query := `
	SELECT spans.id              as id,
		   spans.span_id         as span_id,
		   spans.trace_id        as trace_id,
		   spans.operation_id    as operation_id,
		   spans.flags           as flags,
		   spans.start_time      as start_time,
		   spans.duration        as duration,
		   spans.tags            as tags,
		   spans.service_id      as service_id,
		   spans.process_id      as process_id,
		   spans.process_tags    as process_tags,
		   spans.warnings        as warnings,
		   spans.refs            as refs,
		   spans.logs            as logs,
		   operations.name       as "operation.name",
		   operations.service_id as "operation.service_id",
		   operations.kind       as "operation.kind",
		   services.id           as "service.id",
		   services.name         as "service.name"
	FROM spans
			 INNER JOIN operations on operations.id = spans.operation_id
			 INNER JOIN services on services.id = operations.service_id
	WHERE trace_id = :trace_id
	  AND spans.deleted_at IS NULL
`

	rows, err := r.db.NamedQueryContext(ctx, query, struct {
		TraceId string `db:"trace_id"`
	}{
		TraceId: traceID.String(),
	})

	if err == sql.ErrNoRows {
		return nil, spanstore.ErrTraceNotFound
	}

	if err != nil {
		log.Println("[GetTrace][error] an error occurred while getting trace", err)
		return nil, err
	}

	spans := make([]*model.Span, 0)
	traceProcessingMap := make([]model.Trace_ProcessMapping, 0)
	warnings := make([]string, 0)

	for rows.Next() {
		var internalSpan common.InternalSpan
		if err := rows.StructScan(&internalSpan); err != nil {
			log.Println("[GetTrace][error] an error occurred while calling structScan()", err)
			return nil, err
		}
		span, err := internalSpan.ToSpan()
		if err != nil {
			log.Println("[GetTrace][error] an error occurred while calling ToSpan()", err)
			return nil, err
		}

		tpm := model.Trace_ProcessMapping{
			ProcessID: span.ProcessID,
			Process:   *span.Process,
		}

		spans = append(spans, span)
		traceProcessingMap = append(traceProcessingMap, tpm)
		warnings = append(warnings, span.Warnings...)
	}

	if rows.Err() != nil {
		log.Println("[GetTrace][error] an error in sqlx rows", err)
		return nil, err
	}

	return &model.Trace{
		Spans:      spans,
		ProcessMap: traceProcessingMap,
		Warnings:   warnings,
	}, nil
}

func (r *ReaderDbClient) GetServices(ctx context.Context) ([]string, error) {
	//goland:noinspection ALL
	query := "SELECT name FROM services WHERE deleted_at IS NULL"
	rows, err := r.db.NamedQueryContext(ctx, query, struct{}{})
	if err != nil {
		log.Println("[GetServices][error] an error occurred fetching services", err)
		return nil, err
	}
	services := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Println("[GetServices][error] an error occurred calling scan()", err)
			return nil, err
		}
		services = append(services, name)
	}

	if rows.Err() != nil {
		log.Println("[GetServices][error] an error occurred in rows", err)
		return nil, rows.Err()
	}

	return services, nil
}

func (r *ReaderDbClient) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	//goland:noinspection ALL
	selectQuery := "SELECT o.name, o.kind FROM operations o INNER JOIN services s ON o.service_id = s.id WHERE s.name = :name AND o.deleted_at IS NULL"

	rows, err := r.db.NamedQueryContext(ctx, selectQuery, struct {
		Kind string `db:"kind"`
		Name string `db:"name"`
	}{
		Kind: query.SpanKind,
		Name: query.ServiceName,
	})

	if err != nil {
		log.Println("[GetOperations][error] an error occurred", err)
		return nil, err
	}

	operations := make([]spanstore.Operation, 0)
	for rows.Next() {
		s := struct {
			Name string `db:"name"`
			Kind string `db:"kind"`
		}{}
		if err := rows.StructScan(&s); err != nil {
			return nil, err
		}

		if query.SpanKind == "" || query.SpanKind == s.Kind {
			operations = append(operations, spanstore.Operation{
				Name:     s.Name,
				SpanKind: s.Kind,
			})
		}
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return operations, nil
}

func (r *ReaderDbClient) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	//TODO: implement
	log.Println(fmt.Sprintf("[FindTraces] received a request, query: %+v", query))
	log.Println("[duration]", query.DurationMin, query.DurationMax)
	return []*model.Trace{}, nil
}

func (r *ReaderDbClient) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	//TODO: implement
	log.Println(fmt.Sprintf("[FindTraceIDs] received a request, query: %+v", query))
	return []model.TraceID{}, nil
}
