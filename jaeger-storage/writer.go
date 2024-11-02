package main

import (
	"context"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"jaeger-storage/common"
	"jaeger-storage/storage"
	"log"
	"os"
	"time"
)

type WriterClient struct {
	db          *sqlx.DB
	neo4jWriter *storage.Neo4jWriter
}

func NewWriterDBClient(db *sqlx.DB, neo4jWriter *storage.Neo4jWriter) *WriterClient {
	return &WriterClient{
		db:          db,
		neo4jWriter: neo4jWriter,
	}
}

func (c *WriterClient) upsertService(ctx context.Context, p common.InternalService) (int64, error) {
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

func (c *WriterClient) insertOperation(ctx context.Context, p common.InternalOperation) (int64, error) {
	//goland:noinspection ALL
	query := "WITH new_operation AS (INSERT INTO operations(name, service_id, kind, created_at) values ($1, $2, $3, $4) ON CONFLICT(name, kind, service_id) DO NOTHING RETURNING id) SELECT COALESCE((SELECT id FROM new_operation), (SELECT id from operations WHERE name = $5 AND kind = $6 AND service_id = $7 AND deleted_at IS NULL)) as id"

	var id int64

	err := c.db.GetContext(ctx, &id, query, p.Name, p.ServiceId, p.Kind, p.CreatedAt, p.Name, p.Kind, p.ServiceId)

	return id, err
}

func (c *WriterClient) insertSpan(ctx context.Context, p common.InternalSpan) (int64, error) {
	//goland:noinspection ALL
	query := "INSERT INTO spans(span_id, trace_id, operation_id, flags, start_time, duration, tags, service_id, process_id, process_tags, warnings, logs, kind, refs, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING id"
	var id int64
	err := c.db.GetContext(ctx, &id, query, p.SpanId, p.TraceId, p.OperationId, p.Flags, p.StartTime, p.Duration.Seconds(), p.Tags, p.ServiceId, p.ProcessId, p.ProcessTags, p.WarningsPq, p.Logs, p.Kind, p.Refs, p.CreatedAt)

	return id, err
}

func (c *WriterClient) writeToPostgres(ctx context.Context, span *model.Span, tags, processTags, logs, references []byte) error {
	//	upsert InternalService
	serviceId, err := c.upsertService(ctx, common.InternalService{
		Name:      span.Process.GetServiceName(),
		CreatedAt: time.Now(),
	})
	if err != nil {
		log.Println("[writespan][error] cannot upsert InternalService", err)
		return err
	}
	spanKind, _ := span.GetSpanKind()
	//	upsert operation
	operationId, err := c.insertOperation(ctx, common.InternalOperation{
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
	spanId, err := c.insertSpan(ctx, spanData)
	if err != nil {
		log.Println("[writespan][error] an error occurred while inserting span", err)
		log.Println(fmt.Sprintf("span data %+v", spanData))
		return err
	}
	log.Println(fmt.Sprintf("[writespan] successfully inserted span with primary key: %d, spanId: %s, serviceName: %s, operationName: %s", spanId, span.SpanID.String(), span.Process.GetServiceName(), span.GetOperationName()))
	return nil
}

func (c *WriterClient) WriteSpan(ctx context.Context, span *model.Span) error {
	if span.Process.GetServiceName() == "jaeger-all-in-one" {
		//log.Println("skipping jaeger-all-in-one")
		return nil
	}

	f3, err := os.OpenFile("writer-all.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	defer f3.Close()
	msg := fmt.Sprintf("[writespan] received a request to write a span, spanId: %s, serviceName: %s, operationName: %s\n", span.SpanID.String(), span.Process.GetServiceName(), span.GetOperationName())
	log.Print(msg)
	f3.WriteString(msg)

	tags, err := common.EncodeTags(span.Tags)
	if err != nil {
		log.Println("[writespan][error] an error occurred while encoding tags", err)
		return err
	}
	processTags, err := common.EncodeTags(span.Process.Tags)
	if err != nil {
		log.Println("[writespan][error] an error occurred while encoding process tags", err)
		return err
	}

	logs, internalLogs, err := common.EncodeLogs(span.Logs)
	if err != nil {
		log.Println("[writespan][error] an error occurred while encoding logs", err)
		return err
	}

	references, internalRefs, err := common.EncodeReferences(span.References)
	if err != nil {
		log.Println("[writespan][error] an error occurred while encoding references", err)
		return err
	}

	errChan := make(chan error)

	go func() { errChan <- c.writeToPostgres(ctx, span, tags, processTags, logs, references) }()
	go func() { errChan <- c.neo4jWriter.WriteSpan(ctx, span, internalRefs, internalLogs) }()
	var accumulatedErr string
	for i := 0; i < 2; i++ {
		e := <-errChan
		if e != nil {
			accumulatedErr += e.Error()
		}
	}

	if accumulatedErr != "" {
		return fmt.Errorf(accumulatedErr)
	}

	return nil
}
