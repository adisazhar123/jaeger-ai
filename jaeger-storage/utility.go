package main

import (
	"encoding/base64"
	"encoding/json"
	"github.com/jaegertracing/jaeger/model"
	"log"
)

// adapted from https://stackoverflow.com/questions/15334220/encode-decode-base64
func base64Encode(in []byte) string {
	return base64.StdEncoding.EncodeToString(in)
}

// adapted from https://stackoverflow.com/questions/15334220/encode-decode-base64
func base64Decode(str string) ([]byte, bool) {
	data, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return []byte{}, true
	}
	return data, false
}

func encodeKeyValue(kv model.KeyValue) *InternalKeyValue {
	return &InternalKeyValue{
		Key:       kv.Key,
		Value:     kv.Value(),
		ValueType: kv.VType,
	}
}

func encodeLogs(logs []model.Log) ([]byte, error) {
	internalLogs := make([]InternalLog, len(logs))

	for i := 0; i < len(logs); i++ {
		l := logs[i]
		fields := make([]InternalKeyValue, len(l.Fields))
		for j := 0; j < len(l.Fields); j++ {
			field := l.Fields[j]
			newKv := encodeKeyValue(field)
			fields[j] = *newKv
		}

		internalLog := InternalLog{
			Timestamp: l.GetTimestamp(),
			Fields:    fields,
		}
		internalLogs[i] = internalLog
	}

	return json.Marshal(internalLogs)
}

func decodeLogs(data []byte) ([]model.Log, error) {
	var internalLogs []InternalLog
	if err := json.Unmarshal(data, &internalLogs); err != nil {
		return nil, err
	}
	decodedLogs := make([]model.Log, len(internalLogs))
	for i := 0; i < len(internalLogs); i++ {
		internalLog := internalLogs[i]

		decodedFields := make([]model.KeyValue, len(internalLog.Fields))
		for j := 0; j < len(internalLog.Fields); j++ {
			field := internalLog.Fields[j]
			decodedField := field.ToKeyValue()
			decodedFields = append(decodedFields, decodedField)
		}

		newLog := model.Log{
			Timestamp: internalLog.Timestamp,
			Fields:    decodedFields,
		}
		decodedLogs[i] = newLog
	}

	return decodedLogs, nil
}

func encodeReferences(references []model.SpanRef) ([]byte, error) {
	internalReferences := make([]InternalSpanRef, len(references))
	for i := 0; i < len(references); i++ {
		ref := references[i]
		internalRef := InternalSpanRef{
			TraceId: ref.TraceID.String(),
			SpanId:  ref.SpanID.String(),
			RefType: int64(ref.SpanID),
		}
		internalReferences[i] = internalRef
	}

	return json.Marshal(internalReferences)
}

func decodeReferences(data []byte) ([]model.SpanRef, error) {
	var internalRefs []InternalSpanRef
	if err := json.Unmarshal(data, &internalRefs); err != nil {
		return nil, err
	}
	decodedRefs := make([]model.SpanRef, len(internalRefs))
	for i := 0; i < len(internalRefs); i++ {
		internalRef := internalRefs[i]
		traceId, err := model.TraceIDFromString(internalRef.TraceId)
		if err != nil {
			return nil, err
		}

		spanId, err := model.SpanIDFromString(internalRef.SpanId)
		if err != nil {
			return nil, err
		}

		decodedRef := model.SpanRef{
			TraceID: traceId,
			SpanID:  spanId,
			RefType: model.SpanRefType(internalRef.RefType),
		}
		decodedRefs[i] = decodedRef
	}

	return decodedRefs, nil
}

func encodeTags(tags []model.KeyValue) ([]byte, error) {
	internalTags := make([]InternalKeyValue, len(tags))

	for i := 0; i < len(tags); i++ {
		tag := tags[i]
		t := encodeKeyValue(tag)
		internalTags[i] = *t
	}

	encodedTags, err := json.Marshal(internalTags)
	if err != nil {
		log.Println("[error][encodetags] cannot marshal tags to json", encodedTags)
		return nil, err
	}

	return encodedTags, nil
}

func decodeTags(data []byte) ([]model.KeyValue, error) {
	var internalTags []InternalKeyValue
	if err := json.Unmarshal(data, &internalTags); err != nil {
		return nil, err
	}
	tags := make([]model.KeyValue, len(internalTags))

	for i := 0; i < len(internalTags); i++ {
		internalTag := internalTags[i]
		tags[i] = internalTag.ToKeyValue()
	}

	return tags, nil
}
