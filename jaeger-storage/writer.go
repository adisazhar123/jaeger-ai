package main

import (
	"context"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	_ "github.com/lib/pq"
	"jaeger-storage/common"
	"jaeger-storage/storage"
	"log"
)

type WriterClient struct {
	neo4jWriter *storage.Neo4jWriter
	sqlWriter   *storage.SqlWriter
}

func NewWriterClient(sqlWriter *storage.SqlWriter, neo4jWriter *storage.Neo4jWriter) *WriterClient {
	return &WriterClient{
		sqlWriter:   sqlWriter,
		neo4jWriter: neo4jWriter,
	}
}

func (c *WriterClient) WriteSpan(ctx context.Context, span *model.Span) error {
	if span.Process.GetServiceName() == "jaeger-all-in-one" {
		//log.Println("skipping jaeger-all-in-one")
		return nil
	}

	//f3, err := os.OpenFile("writer-all.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	//defer f3.Close()
	msg := fmt.Sprintf("[writespan] received a request to write a span, spanId: %s, serviceName: %s, operationName: %s\n", span.SpanID.String(), span.Process.GetServiceName(), span.GetOperationName())
	log.Print(msg)
	//f3.WriteString(msg)

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

	go func() { errChan <- c.sqlWriter.WriteSpan(ctx, span, tags, processTags, logs, references) }()
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
