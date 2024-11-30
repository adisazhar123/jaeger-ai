package main

import (
	"context"
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
	"golang.org/x/exp/slices"
	"jaeger-storage/clients"
	"jaeger-storage/common"
	"log"
	"math"
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

		limitInt, _ := strconv.Atoi(limit)

		getTraceIdFulltextSearchQuery := `
			call db.index.fulltext.queryNodes('span_summary_fulltext', $query, { limit: $limit })
			yield node as n, score
			with n, score
			match (n)<-[r:CONTAINS]-(t: Trace)
			return t.trace_id as trace_id, score
		`

		param := map[string]any{
			"limit": limitInt,
			"query": q,
		}

		res, err := neo4j.ExecuteQuery(context, *neo4jDriver, getTraceIdFulltextSearchQuery, param, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
		if err != nil {
			log.Println("[search][ExecuteQuery] error occurred", err)
			context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}
		minScore := math.MaxFloat64
		maxScore := float64(math.MinInt64)
		fetchedTraces := make([]struct {
			score   float64
			traceId string
		}, 0)

		for _, record := range res.Records {
			tid, _ := record.Get("trace_id")
			s, _ := record.Get("score")
			score := s.(float64)
			traceId := tid.(string)

			log.Printf("[fulltext] traceid %s score %f\n", traceId, score)
			fetchedTraces = append(fetchedTraces, struct {
				score   float64
				traceId string
			}{score: score, traceId: traceId})

			minScore = math.Min(minScore, score)
			maxScore = math.Max(maxScore, score)
		}

		for i := 0; i < len(fetchedTraces); i++ {
			fetchedTraces[i].score = (fetchedTraces[i].score - minScore) / (maxScore - minScore)
		}

		log.Println("minScore", minScore, "maxScore", maxScore)

		log.Println("fetchedTraces after normalizing score", fetchedTraces)

		// get embedding
		embedding, err := openaiClient.CreateEmbeddings(context, q)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		// vector search neo4j and get trace id from the nodes
		getTraceIdQuery := `
			call db.index.vector.queryNodes('span_summary', $limit, $embedding)
			yield node as n, score
			match (n)<-[r:CONTAINS]-(t: Trace)
			return t.trace_id as trace_id, score
		`
		param = map[string]any{
			"limit":     limitInt,
			"embedding": embedding,
		}

		res, err = neo4j.ExecuteQuery(context, *neo4jDriver, getTraceIdQuery, param, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
		if err != nil {
			log.Println("[search][ExecuteQuery] error occurred", err)
			context.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		for _, record := range res.Records {
			tid, _ := record.Get("trace_id")
			s, _ := record.Get("score")
			traceId := tid.(string)
			score := s.(float64)

			log.Printf("[vector] traceid %s score %f\n", traceId, score)

			fetchedTraces = append(fetchedTraces, struct {
				score   float64
				traceId string
			}{score: score, traceId: traceId})
		}

		// fetch traces by trace id from postgres
		getTracesQuery := `
			SELECT spans.id as id, spans.span_id as span_id, spans.trace_id as trace_id, spans.operation_id as operation_id, spans.flags as flags, spans.start_time as start_time, extract(epoch from spans.duration) as duration, spans.tags as tags, spans.service_id as service_id, spans.process_id as process_id, spans.process_tags as process_tags, spans.warnings as warnings, spans.refs as refs, spans.logs as logs, operation.name as "operation.name", operation.service_id as "operation.service_id", operation.kind as "operation.kind", service.id as "service.id", service.name as "service.name"
			FROM spans
			 INNER JOIN services service on spans.service_id = service.id
			 INNER JOIN operations operation on spans.operation_id = operation.id
			WHERE spans.trace_id IN (?) and spans.deleted_at IS NULL
	`

		slices.SortFunc(fetchedTraces, func(a, b struct {
			score   float64
			traceId string
		}) int {
			if a.score < b.score {
				return 1
			} else if a.score > b.score {
				return -1
			}
			return 0
		})

		traceIdsToScore := make(map[string]float64)
		filteredTraceIds := make([]string, 0)

		for _, v := range fetchedTraces {
			if _, ok := traceIdsToScore[v.traceId]; !ok {
				traceIdsToScore[v.traceId] = v.score
				filteredTraceIds = append(filteredTraceIds, v.traceId)
			}
		}

		if len(filteredTraceIds) > limitInt {
			filteredTraceIds = filteredTraceIds[:limitInt]
		}

		getTracesQuery, args, err := sqlx.In(getTracesQuery, filteredTraceIds)
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
			//log.Println("duration custom", internalSpan.DurationCustom)
			//data := binary.BigEndian.Uint64(internalSpan.DurationCustom)
			//log.Println("duration", data)
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

	r.POST("/api/ask", func(c *gin.Context) {
		type askRequest struct {
			TraceId  string `json:"trace_id"`
			Question string `json:"question"`
			Hop      int    `json:"hop"`
		}
		// get trace_id (optional)
		req := askRequest{}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}
		// if trace_id exists, filter span by trace_id
		// perform vector search, from that starting node, aggregate k hops
		ctx := context.Background()
		embedding, err := openaiClient.CreateEmbeddings(ctx, req.Question)
		if err != nil {
			log.Println("[/ask][CreateEmbeddings] an error occurred", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		query := `
			MATCH (s: Span)<-[r:CONTAINS]-(t: Trace {trace_id: $traceId})
			WITH s, vector.similarity.cosine(s.embedding, $embedding) AS score
			RETURN s.span_id as span_id, score
			ORDER BY score DESC
			LIMIT 1
		`

		param := map[string]any{
			"embedding": embedding,
			"traceId":   req.TraceId,
		}

		res, err := neo4j.ExecuteQuery(ctx, *neo4jDriver, query, param, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
		if err != nil {
			log.Println("[search][ExecuteQuery] error occurred", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		spanId, _, err := neo4j.GetRecordValue[string](res.Records[0], "span_id")
		log.Println(spanId)
		if err != nil {
			log.Println("[search][GetRecordValue] error occurred", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		// TODO: find starting node, replace with span_id param
		query2 := fmt.Sprintf("MATCH p=(s:Span{span_id: $span_id})-[r:INVOKES_CHILD|INVOKES_FOLLOWS*%d]-(s2:Span) return p", req.Hop)
		param = map[string]any{
			"span_id": spanId,
		}
		res, err = neo4j.ExecuteQuery(ctx, *neo4jDriver, query2, param, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase("neo4j"))
		if err != nil {
			log.Println("[search][ExecuteQuery] error occurred", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}
		visited := make(map[string]struct{})
		//
		//var passage string
		var nodes = "Nodes\n"
		var edges = "Edges\n"

		// TODO: build the passage from the paths
		for _, record := range res.Records {
			path, _, _ := neo4j.GetRecordValue[neo4j.Path](record, "p")
			var relationship string
			for _, rel := range path.Relationships {
				startNode := findNode(rel.StartElementId, path.Nodes)
				endNode := findNode(rel.EndElementId, path.Nodes)

				startSpanId, _ := neo4j.GetProperty[string](startNode, "span_id")
				endSpanId, _ := neo4j.GetProperty[string](endNode, "span_id")
				relationship += fmt.Sprintf("(%s, %s, %s)\n", startSpanId, rel.Type, endSpanId)
			}
			edges += relationship

			var localNode string
			for _, node := range path.Nodes {
				if _, ok := visited[node.ElementId]; !ok {
					visited[node.ElementId] = struct{}{}
					sId, _ := neo4j.GetProperty[string](node, "span_id")
					summary, _ := neo4j.GetProperty[string](node, "summary")
					localNode += fmt.Sprintf("Span ID: %s\nSummary: %s\n", sId, summary)
				}
			}
			nodes += localNode
		}
		passage := edges + "\n" + nodes
		answer, err := openaiClient.GenerateAnswer(ctx, req.Question, passage)

		if err != nil {
			log.Println("an error occurred while generating an answer", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, err.Error())
			return
		}

		c.JSON(http.StatusOK, struct {
			Answer  string `json:"answer"`
			Passage string `json:"passage"`
		}{
			Answer:  answer,
			Passage: passage,
		})

		//c.String(http.StatusOK, fmt.Sprintf("answer: %s\npassage: %s", answer, passage))
		return
		// else if trace_id does not exist
		// perform vector search, from that starting node, aggregate k hops
	})

	return r
}

func findNode(elementId string, nodes []neo4j.Node) *neo4j.Node {
	for _, v := range nodes {
		if v.ElementId == elementId {
			return &v
		}
	}
	return nil
}
