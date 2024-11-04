package storage

import (
	"context"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"jaeger-storage/common"
	"log"
	"sync"
)

type relationshipSpan struct {
	relationship string
	childSpanId  string
}

type Neo4jWriter struct {
	driver         *neo4j.DriverWithContext
	missingParents map[string][]relationshipSpan
	mutex          sync.Mutex
}

func NewNeo4jWriter(driver *neo4j.DriverWithContext) *Neo4jWriter {
	return &Neo4jWriter{
		driver:         driver,
		missingParents: make(map[string][]relationshipSpan),
		mutex:          sync.Mutex{},
	}
}

func (w *Neo4jWriter) upsertServiceTraceSpan(ctx context.Context, span *model.Span) error {
	neo4jQuery := `
			MERGE (service: Service {name: $service_name})
			MERGE (trace: Trace { trace_id: $trace_id, summary: $trace_summary })
			MERGE (span: Span {
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
			MERGE (service)-[r_contain:CONTAINS]->(span)
			MERGE (trace)-[r_contain_span:CONTAINS]->(span)
			RETURN (span)
		`
	spanKind, _ := span.GetSpanKind()
	param := map[string]any{
		"service_name":   span.Process.GetServiceName(),
		"operation_name": span.GetOperationName(),
		"span_id":        span.SpanID.String(),
		"duration":       span.Duration.Nanoseconds(),
		"start_time":     span.StartTime,
		"log_summary":    "TODO: empty-for-now",
		"tag_summary":    "TODO: empty-for-now",
		"trace_summary":  "TODO: empty-for-now",
		"span_kind":      spanKind.String(),
		// TODO: lookup from tags/logs
		"action_kind":        "http",
		"action_status_code": "TODO: empty for now",
		"trace_id":           span.TraceID.String(),
	}
	_, err := neo4j.ExecuteQuery(ctx, *w.driver, neo4jQuery, param, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
	if err != nil {
		log.Printf("[error][neo4j][upsertServiceTraceSpan] cannot create service and span err: %s, param: %+v\n", err, param)
		return err
	}

	log.Printf("[neo4j][upsertServiceTraceSpan] successfully upsert span with ID %s\n", span.SpanID.String())

	return nil
}

func (w *Neo4jWriter) insertLogs(ctx context.Context, span *model.Span, internalLogs []common.InternalLog) error {
	createLogsQuery := `
			MATCH(span: Span { span_id: $span_id })
			CREATE (n: Log { value: $value, timestamp: $timestamp })<-[r:PRODUCES]-(span)	
		`
	for i := 0; i < len(internalLogs); i++ {
		l := internalLogs[i]
		var value string
		for j := 0; j < len(l.Fields); j++ {
			currentLog := l.Fields[j]

			value += fmt.Sprintf("%s: %s\n", currentLog.Key, currentLog.Value)
		}

		_, err := neo4j.ExecuteQuery(ctx, *w.driver, createLogsQuery, map[string]any{
			"span_id":   span.SpanID.String(),
			"value":     value,
			"timestamp": l.Timestamp,
		}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))

		if err != nil {
			log.Println("[neo4j][insertLogs][error] cannot create logs", err)
			return err
		}
	}

	log.Println("[neo4j][insertLogs] successfully inserted logs")

	return nil
}

func (w *Neo4jWriter) createRelationshipBetweenSpan(ctx context.Context, span *model.Span, internalRefs []common.InternalSpanRef) error {
	for i := 0; i < len(internalRefs); i++ {
		log.Printf("[neo4j][createRelationshipBetweenSpan] associating span child: %s with span parent: %s", span.SpanID.String(), span.ParentSpanID())
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

		associateChildParentResult, err := neo4j.ExecuteQuery(ctx, *w.driver, q, map[string]any{
			"span_id_child":  span.SpanID.String(),
			"span_id_parent": r.SpanId,
		}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
		if err != nil {
			log.Printf("[neo4j][createRelationshipBetweenSpan][error] cannot associate spans, child span id: %s, parent span id: %s, err: %s\n", span.SpanID.String(), r.SpanId, err)
			return err
		}

		missingParent := len(associateChildParentResult.Records) == 0

		if missingParent {
			w.mutex.Lock()
			_, ok := w.missingParents[r.SpanId]
			if !ok {
				w.missingParents[r.SpanId] = make([]relationshipSpan, 0)
			}
			w.missingParents[r.SpanId] = append(w.missingParents[r.SpanId], relationshipSpan{
				relationship: relationShip,
				childSpanId:  span.SpanID.String(),
			})
			w.mutex.Unlock()
		} else {
			log.Printf("[neo4j][createRelationshipBetweenSpan] successfully associated span child: %s with span parent: %s", span.SpanID.String(), span.ParentSpanID())
		}
	}

	return nil
}

func (w *Neo4jWriter) associateMissingSpan(ctx context.Context, span *model.Span) error {
	w.mutex.Lock()
	missingSpans, _ := w.missingParents[span.SpanID.String()]
	for i := 0; i < len(missingSpans); i++ {
		missingSpan := missingSpans[i]

		q := fmt.Sprintf(`
				match(span_child: Span { span_id: $span_id_child })
				match(span_parent: Span { span_id: $span_id_parent })
				merge (span_parent)-[r_invoke:%s]->(span_child)
				return span_parent
			`, missingSpan.relationship)
		_, err := neo4j.ExecuteQuery(ctx, *w.driver, q, map[string]any{
			"span_id_child":  missingSpan.childSpanId,
			"span_id_parent": span.SpanID.String(),
		}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))

		if err != nil {
			log.Println("[neo4j][associateMissingSpan][error] cannot associate missing spans", err)
			return err
		}

		log.Printf("[neo4j][associateMissingSpan] fixed parent id: %s, child id: %s", span.SpanID.String(), missingSpan.childSpanId)
	}
	delete(w.missingParents, span.SpanID.String())
	w.mutex.Unlock()
	return nil
}

func (w *Neo4jWriter) WriteSpan(ctx context.Context, span *model.Span, internalRefs []common.InternalSpanRef, internalLogs []common.InternalLog) error {
	if span.Process.GetServiceName() != "jaeger-all-in-one" {
		if err := w.upsertServiceTraceSpan(ctx, span); err != nil {
			return err
		}

		if err := w.insertLogs(ctx, span, internalLogs); err != nil {
			return err
		}

		if err := w.createRelationshipBetweenSpan(ctx, span, internalRefs); err != nil {
			return err
		}

		if err := w.associateMissingSpan(ctx, span); err != nil {
			return err
		}
	}
	return nil
}
