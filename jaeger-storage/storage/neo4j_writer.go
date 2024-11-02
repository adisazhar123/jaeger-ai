package storage

import (
	"context"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"jaeger-storage/common"
	"log"
	"os"
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

func (w *Neo4jWriter) WriteSpan(ctx context.Context, span *model.Span, internalRefs []common.InternalSpanRef, internalLogs []common.InternalLog) error {
	if span.Process.GetServiceName() != "jaeger-all-in-one" {
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
		spanNodeResult, err := neo4j.ExecuteQuery(ctx, *w.driver, neo4jQuery, param, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
		if err != nil {
			log.Println("[error][neo4j] cannot create service and span", err)
			log.Println("param is", param)
			return err
		}

		log.Println("[spanNodeResult]", spanNodeResult.Summary.Query())

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

			createLogsResult, err := neo4j.ExecuteQuery(ctx, *w.driver, createLogsQuery, map[string]any{
				"span_id":   span.SpanID.String(),
				"value":     value,
				"timestamp": l.Timestamp,
			}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))

			if err != nil {
				log.Println("[error][neo4j] cannot create logs", err)
				return err
			}

			log.Println("[createLogsResult]", createLogsResult.Summary.Query())
		}

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

			associateChildParentResult, err := neo4j.ExecuteQuery(ctx, *w.driver, q, map[string]any{
				"span_id_child":  span.SpanID.String(),
				"span_id_parent": r.SpanId,
			}, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
			if err != nil {
				log.Println("[error][neo4j] cannot associate spans", err)
				return err
			}

			if len(associateChildParentResult.Records) == 0 {
				w.mutex.Lock()
				_, ok := w.missingParents[r.SpanId]

				if !ok {
					w.missingParents[r.SpanId] = make([]relationshipSpan, 0)
				}

				w.missingParents[r.SpanId] = append(w.missingParents[r.SpanId], struct {
					relationship string
					childSpanId  string
				}{relationship: relationShip, childSpanId: span.SpanID.String()})
				w.mutex.Unlock()

				msg := fmt.Sprintf("[missing-parent] span %s cannot find parent span %s\n", span.SpanID.String(), r.SpanId)
				if _, err = f.WriteString(msg); err != nil {
					log.Println("cannot write to file", err)
				}
			} else {
				log.Println(fmt.Sprintf("[found-parent] span %s found parent span %s", span.SpanID.String(), r.SpanId))
			}
		}

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
				log.Println("[error][neo4j] cannot associate missing spans", err)
				return err
			}
			f2.WriteString(fmt.Sprintf("fixing parent span %s and child span %s\n", span.SpanID.String(), missingSpan.childSpanId))
		}
		delete(w.missingParents, span.SpanID.String())
		w.mutex.Unlock()
	}
	return nil
}
