package main

import (
	"context"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"log"
	"os"
	"sync"
	"time"
)

type relationshipSpan struct {
	relationship string
	childSpanId  string
}

type WriterDbClient struct {
	db             *sqlx.DB
	driver         *neo4j.DriverWithContext
	enableNeo4j    bool
	missingParents map[string][]relationshipSpan
	mutex          sync.Mutex
}

func NewWriterDBClient(db *sqlx.DB, driver *neo4j.DriverWithContext, enableNeo4j bool) *WriterDbClient {
	return &WriterDbClient{
		db:             db,
		driver:         driver,
		enableNeo4j:    enableNeo4j,
		missingParents: make(map[string][]relationshipSpan),
		mutex:          sync.Mutex{},
	}
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
	if span.Process.GetServiceName() == "jaeger-all-in-one" {
		//log.Println("skipping jaeger-all-in-one")
		return nil
	}

	f3, err := os.OpenFile("writer-all.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	defer f3.Close()
	msg := fmt.Sprintf("[writespan] received a request to write a span, spanId: %s, serviceName: %s, operationName: %s\n", span.SpanID.String(), span.Process.GetServiceName(), span.GetOperationName())
	log.Print(msg)
	f3.WriteString(msg)

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

	references, internalRefs, err := encodeReferences(span.References)
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

	if c.enableNeo4j && span.Process.GetServiceName() != "jaeger-all-in-one" {
		// neo4j stuff
		neo4jQuery := `
	merge (service: Service {name: $service_name})
	create (span: Span {
		operation_name: $operation_name,
		span_id: $span_id,
		duration: $duration,
		start_time: $start_time,
		log_summary: $log_summary,
		tag_summary: $tag_summary,
		span_kind: $span_kind,
		action_kind: $action_kind,
		action_status_code: $action_status_code
	})
	merge (service)-[r_contain:CONTAINS]->(span)
	return (span)
`
		spanNodeResult, err := neo4j.ExecuteQuery(ctx, *c.driver, neo4jQuery, map[string]any{
			"service_name":   span.Process.GetServiceName(),
			"operation_name": span.GetOperationName(),
			"span_id":        span.SpanID.String(),
			"duration":       span.Duration.Nanoseconds(),
			"start_time":     span.StartTime,
			"log_summary":    "TODO: empty-for-now",
			"tag_summary":    "TODO: empty-for-now",
			"span_kind":      spanKind.String(),
			// TODO: lookup from tags/logs
			"action_kind":        "http",
			"action_status_code": "TODO: empty for now",
		}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
		if err != nil {
			log.Println("[error][neo4j] cannot create service and span", err)
			return err
		}

		log.Println("[spanNodeResult]", spanNodeResult.Summary.Query())

		f, err := os.OpenFile("writer-missing.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		defer f.Close()

		f2, err := os.OpenFile("writer-fix.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		defer f2.Close()

		for i := 0; i < len(internalRefs); i++ {
			log.Printf("[info] associating span child: %s with span parent: %s", span.SpanID.String(), span.ParentSpanID())
			r := internalRefs[i]
			relationShip := "INVOKES_CHILD"
			if r.RefType == uint64(model.SpanRefType_FOLLOWS_FROM) {
				relationShip = "INVOKES_FOLLOWS"
			}
			q := fmt.Sprintf(`
				match(span_child: Span { span_id: $span_id_child })
				match(span_parent: Span { span_id: $span_id_parent })
				merge (span_parent)-[r_invoke:%s]->(span_child)
				return span_parent
			`, relationShip)

			associateChildParentResult, err := neo4j.ExecuteQuery(ctx, *c.driver, q, map[string]any{
				"span_id_child":  span.SpanID.String(),
				"span_id_parent": r.SpanId,
			}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
			if err != nil {
				log.Println("[error][neo4j] cannot associate spans", err)
				return err
			}

			if len(associateChildParentResult.Records) == 0 {
				c.mutex.Lock()
				_, ok := c.missingParents[r.SpanId]

				if !ok {
					c.missingParents[r.SpanId] = make([]relationshipSpan, 0)
				}

				c.missingParents[r.SpanId] = append(c.missingParents[r.SpanId], struct {
					relationship string
					childSpanId  string
				}{relationship: relationShip, childSpanId: span.SpanID.String()})
				c.mutex.Unlock()

				msg := fmt.Sprintf("[missing-parent] span %s cannot find parent span %s\n", span.SpanID.String(), r.SpanId)
				if _, err = f.WriteString(msg); err != nil {
					log.Println("cannot write to file", err)
				}
			} else {
				log.Println(fmt.Sprintf("[found-parent] span %s found parent span %s", span.SpanID.String(), r.SpanId))
			}
		}

		c.mutex.Lock()
		missingSpans, _ := c.missingParents[span.SpanID.String()]
		for i := 0; i < len(missingSpans); i++ {
			missingSpan := missingSpans[i]

			q := fmt.Sprintf(`
				match(span_child: Span { span_id: $span_id_child })
				match(span_parent: Span { span_id: $span_id_parent })
				merge (span_parent)-[r_invoke:%s]->(span_child)
				return span_parent
			`, missingSpan.relationship)
			_, err := neo4j.ExecuteQuery(ctx, *c.driver, q, map[string]any{
				"span_id_child":  missingSpan.childSpanId,
				"span_id_parent": span.SpanID.String(),
			}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))

			if err != nil {
				log.Println("[error][neo4j] cannot associate missing spans", err)
				return err
			}
			f2.WriteString(fmt.Sprintf("fixing parent span %s and child span %s\n", span.SpanID.String(), missingSpan.childSpanId))
		}
		delete(c.missingParents, span.SpanID.String())
		c.mutex.Unlock()
	}

	return nil
}
