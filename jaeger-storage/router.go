package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	uiconv "github.com/jaegertracing/jaeger/model/converter/json"
	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jmoiron/sqlx"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"jaeger-storage/clients"
	"jaeger-storage/common"
	"log"
	"net/http"
	"strconv"
	"time"
)

type structuredError struct {
	Code    int        `json:"code,omitempty"`
	Msg     string     `json:"msg"`
	TraceID ui.TraceID `json:"traceID,omitempty"`
}

type structuredResponse struct {
	Data   any               `json:"data"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
	Errors []structuredError `json:"errors"`
}

var jaegerAdjuster = adjuster.Sequence(querysvc.StandardAdjusters(time.Second)...)

// helper function copied from Jaeger library
func tracesToResponse(traces []*model.Trace, adjust bool, uiErrors []structuredError) *structuredResponse {
	uiTraces := make([]*ui.Trace, len(traces))
	for i, v := range traces {
		uiTrace, uiErr := convertModelToUI(v, adjust)
		if uiErr != nil {
			uiErrors = append(uiErrors, *uiErr)
		}
		uiTraces[i] = uiTrace
	}

	return &structuredResponse{
		Data:   uiTraces,
		Errors: uiErrors,
	}
}

// helper function copied from Jaeger library
func convertModelToUI(trace *model.Trace, adjust bool) (*ui.Trace, *structuredError) {
	var errs []error
	if adjust {
		var err error
		trace, err = jaegerAdjuster.Adjust(trace)
		if err != nil {
			errs = append(errs, err)
		}
	}
	uiTrace := uiconv.FromDomain(trace)
	var uiError *structuredError
	if err := errors.Join(errs...); err != nil {
		uiError = &structuredError{
			Msg:     err.Error(),
			TraceID: uiTrace.TraceID,
		}
	}
	return uiTrace, uiError
}

func NewRouter(openaiClient *clients.OpenAIClient, neo4jDriver *neo4j.DriverWithContext, db *sqlx.DB) *gin.Engine {
	r := gin.Default()

	r.GET("/api/search", func(context *gin.Context) {
		q := context.Query("query")
		limit := context.Query("limit")

		if q == "" || limit == "" {
			context.AbortWithStatusJSON(http.StatusInternalServerError, fmt.Sprintf("please provide limit and query. received limit=%s, query=%s", limit, q))
			return
		}

		// get embedding
		embedding, err := openaiClient.CreateEmbeddings(context, q)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		// vector search neo4j and get trace id from the nodes
		getTraceIdQuery := `
			call db.index.vector.queryNodes('span_summary', $limit, $embedding)
			yield node as n
			match (n)<-[r:CONTAINS]-(t: Trace)
			return distinct t.trace_id as trace_id
		`
		limitInt, _ := strconv.Atoi(limit)
		param := map[string]any{
			"limit":     limitInt,
			"embedding": embedding,
		}

		res, err := neo4j.ExecuteQuery(context, *neo4jDriver, getTraceIdQuery, param, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
		if err != nil {
			log.Println("[search][ExecuteQuery] error occurred", err)
			context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		fmt.Println(res.Records)

		var traceIds []string

		for _, record := range res.Records {
			tid, _ := record.Get("trace_id")
			traceIds = append(traceIds, tid.(string))
		}

		// fetch traces by trace id from postgres
		getTracesQuery := `
		SELECT spans.id as id, spans.span_id as span_id, spans.trace_id as trace_id, spans.operation_id as operation_id, spans.flags as flags, spans.start_time as start_time, extract(epoch from spans.duration) as duration, spans.tags as tags, spans.service_id as service_id, spans.process_id as process_id, spans.process_tags as process_tags, spans.warnings as warnings, spans.refs as refs, spans.logs as logs, operation.name as "operation.name", operation.service_id as "operation.service_id", operation.kind as "operation.kind", service.id as "service.id", service.name as "service.name"
		FROM spans
		 INNER JOIN services service on spans.service_id = service.id
		 INNER JOIN operations operation on spans.operation_id = operation.id
		WHERE spans.trace_id IN (?) and spans.deleted_at IS NULL
`

		getTracesQuery, args, err := sqlx.In(getTracesQuery, traceIds)
		if err != nil {
			log.Println("error sqlx in", err)
			context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		getTracesQuery = db.Rebind(getTracesQuery)
		rows, err := db.QueryxContext(context, getTracesQuery, args...)

		if err != nil {
			log.Println("error sqlx in", err)
			context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		var traces []*model.Trace

		traceToSpans := make(map[string][]*model.Span)
		traceToProcess := make(map[string][]model.Trace_ProcessMapping)
		traceToWarnings := make(map[string][]string)

		for rows.Next() {
			internalSpan := common.InternalSpan{}
			if err := rows.StructScan(&internalSpan); err != nil {
				log.Println("error StructScan", err)
				context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
				return
			}
			log.Println("duration custom", internalSpan.DurationCustom)
			data := binary.BigEndian.Uint64(internalSpan.DurationCustom)
			log.Println("duration", data)
			span, err := internalSpan.ToSpan()
			if err != nil {
				log.Println(err)
				context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
				return
			}
			traceToSpans[internalSpan.TraceId] = append(traceToSpans[internalSpan.TraceId], span)
			traceToProcess[internalSpan.TraceId] = append(traceToProcess[internalSpan.TraceId], model.Trace_ProcessMapping{
				ProcessID: span.ProcessID,
				Process:   *span.GetProcess(),
			})
			traceToWarnings[internalSpan.TraceId] = append(traceToWarnings[internalSpan.TraceId], span.Warnings...)
		}

		if rows.Err() != nil {
			log.Println("error in rows", err)
			context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		for k, v := range traceToSpans {
			jaegerTrace := &model.Trace{
				Spans:      v,
				ProcessMap: traceToProcess[k],
				Warnings:   traceToWarnings[k],
			}
			traces = append(traces, jaegerTrace)
		}
		uiErrors := make([]structuredError, 0)
		context.JSON(http.StatusOK, tracesToResponse(traces, true, uiErrors))
		return
	})

	r.POST("/ask", func(context *gin.Context) {
		// get trace_id (optional)

		// if trace_id exists, filter span by trace_id
		// perform vector search, from that starting node, aggregate k hops

		// else if trace_id does not exist
		// perform vector search, from that starting node, aggregate k hops
	})

	return r
}
