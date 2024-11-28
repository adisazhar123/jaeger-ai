package common

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"log"
	"time"
)

type InternalKeyValue struct {
	Key       string          `json:"key,omitempty"`
	Value     interface{}     `json:"value,omitempty"`
	ValueType model.ValueType `json:"value_type"`
}

func (kv InternalKeyValue) ToKeyValue() model.KeyValue {
	fmt.Printf("key: %s, val: %s\n", kv.Key, kv.Value)
	newKv := model.KeyValue{}
	newKv.Key = kv.Key
	newKv.VType = kv.ValueType
	if kv.ValueType == model.StringType {
		newKv.VStr = kv.Value.(string)
	} else if kv.ValueType == model.ValueType_BOOL {
		newKv.VBool = kv.Value.(bool)
	} else if kv.ValueType == model.ValueType_INT64 {
		// when decoding int64, golang treats numbers as float
		// so we have to check if typecasting to int64 it works
		// otherwise, typecast it to float64 then to int64
		v, ok := kv.Value.(int64)
		if !ok {
			v = int64(kv.Value.(float64))
		}
		newKv.VInt64 = v
	} else if kv.ValueType == model.ValueType_FLOAT64 {
		newKv.VFloat64 = kv.Value.(float64)
	} else if kv.ValueType == model.ValueType_BINARY {
		newKv.VBinary = kv.Value.([]byte)
	} else {
		log.Println(fmt.Sprintf("[decodeKeyValue] unknown type %s", kv.ValueType))
	}

	return newKv
}

type InternalLog struct {
	Timestamp time.Time          `json:"timestamp,omitempty"`
	Fields    []InternalKeyValue `json:"fields,omitempty"`
}

type InternalSpanRef struct {
	TraceId string `json:"trace_id"`
	SpanId  string `json:"span_id"`
	RefType uint64 `json:"ref_type"`
}

type InternalSpan struct {
	Id             int64              `db:"id"`
	SpanId         string             `db:"span_id"`
	TraceId        string             `db:"trace_id"`
	OperationId    int64              `db:"operation_id"`
	Operation      *InternalOperation `db:"operation"`
	Flags          uint64             `db:"flags"`
	StartTime      time.Time          `db:"start_time"`
	Duration       time.Duration      `db:"duration_"`
	DurationCustom []uint8            `db:"duration"`
	Tags           []byte             `db:"tags"`
	ServiceId      int64              `db:"service_id"`
	Service        *InternalService   `db:"service"`
	ProcessId      string             `db:"process_id"`
	ProcessTags    []byte             `db:"process_tags"`
	Warnings       *[]string          `db:"warnings"`
	WarningsPq     interface {
		driver.Valuer
		sql.Scanner
	} `db:"warnings_pq_array"`
	Logs      []byte     `db:"logs"`
	Kind      string     `db:"kind"`
	Refs      []byte     `db:"refs"`
	CreatedAt time.Time  `db:"created_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}

func (s InternalSpan) ToSpan() (*model.Span, error) {
	traceId, err := model.TraceIDFromString(s.TraceId)
	if err != nil {
		return nil, err
	}

	spanId, err := model.SpanIDFromString(s.SpanId)
	if err != nil {
		return nil, err
	}

	var operationName = ""
	if s.Operation != nil {
		operationName = s.Operation.Name
	}

	references, err := DecodeReferences(s.Refs)
	if err != nil {
		return nil, err
	}

	log.Println("decode tags span id", s.SpanId)
	tags, err := DecodeTags(s.Tags)
	if err != nil {
		return nil, err
	}

	logs, err := DecodeLogs(s.Logs)
	if err != nil {
		return nil, err
	}

	process, err := s.getProcess()
	if err != nil {
		return nil, err
	}

	span := &model.Span{
		TraceID:       traceId,
		SpanID:        spanId,
		OperationName: operationName,
		References:    references,
		Flags:         model.Flags(s.Flags),
		StartTime:     s.StartTime,
		Duration:      s.Duration,
		Tags:          tags,
		Logs:          logs,
		Process:       process,
		ProcessID:     s.ProcessId,
	}

	if s.Warnings == nil {
		span.Warnings = []string{}
	} else {
		span.Warnings = *s.Warnings
	}

	return span, nil
}

func (s InternalSpan) getProcess() (*model.Process, error) {
	serviceName := ""
	if s.Service != nil {
		serviceName = s.Service.Name
	}

	tags, err := DecodeTags(s.ProcessTags)
	if err != nil {
		return nil, err
	}

	return &model.Process{
		ServiceName: serviceName,
		Tags:        tags,
	}, nil
}

type InternalOperation struct {
	Name      string     `db:"name"`
	ServiceId int64      `db:"service_id"`
	Kind      string     `db:"kind"`
	CreatedAt time.Time  `db:"created_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}

type InternalService struct {
	Id        int64      `db:"id"`
	Name      string     `db:"name"`
	CreatedAt time.Time  `db:"created_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}
