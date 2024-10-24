package main

import "time"

type InternalKeyValue struct {
	Key   string `json:"key,omitempty"`
	Value any    `json:"value,omitempty"`
}

type InternalLog struct {
	Timestamp time.Time          `json:"timestamp,omitempty"`
	Fields    []InternalKeyValue `json:"fields,omitempty"`
}

type InternalSpanRef struct {
	TraceId string `json:"trace_id"`
	SpanId  string `json:"span_id"`
	RefType int64  `json:"ref_type"`
}

type InternalTag struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type InternalSpan struct {
	Id          int64         `db:"id"`
	SpanId      string        `db:"span_id"`
	TraceId     string        `db:"trace_id"`
	OperationId int64         `db:"operation_id"`
	Flags       int64         `db:"flags"`
	StartTime   time.Time     `db:"start_time"`
	Duration    time.Duration `db:"duration"`
	Tags        []byte        `db:"tags"`
	ServiceId   int64         `db:"service_id"`
	ProcessId   string        `db:"process_id"`
	ProcessTags []byte        `db:"process_tags"`
	Warnings    []string      `db:"warnings"`
	Logs        []byte        `db:"logs"`
	Kind        string        `db:"kind"`
	Refs        []byte        `db:"refs"`
	CreatedAt   time.Time     `db:"created_at"`
	DeletedAt   *time.Time    `db:"deleted_at"`
}

type InternalOperation struct {
	Name      string     `db:"name"`
	ServiceId int64      `db:"service_id"`
	Kind      string     `db:"kind"`
	CreatedAt time.Time  `db:"created_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}
