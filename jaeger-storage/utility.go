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

func extractFromKeyValue(kv model.KeyValue) (string, any) {
	if kv.Value() == model.StringType {
		return kv.Key, kv.GetVStr()
	} else if kv.Value() == model.ValueType_BOOL {
		return kv.Key, kv.GetVBool()
	} else if kv.Value() == model.ValueType_INT64 {
		return kv.Key, kv.GetVInt64()
	} else if kv.Value() == model.ValueType_FLOAT64 {
		return kv.Key, kv.GetVFloat64()
	} else if kv.Value() == model.ValueType_BINARY {
		return kv.Key, base64Encode(kv.GetVBinary())
	} else {
		return kv.Key, "unknown"
	}
}

func encodeLogs(logs []model.Log) ([]byte, error) {
	internalLogs := make([]InternalLog, len(logs))

	for i := 0; i < len(logs); i++ {
		log := logs[i]
		fields := make([]InternalKeyValue, len(log.Fields))
		for j := 0; j < len(log.Fields); j++ {
			field := log.Fields[j]
			key, value := extractFromKeyValue(field)
			newField := InternalKeyValue{
				Key:   key,
				Value: value,
			}
			fields[i] = newField
		}

		internalLog := InternalLog{
			Timestamp: log.GetTimestamp(),
			Fields:    fields,
		}
		internalLogs[i] = internalLog
	}

	return json.Marshal(internalLogs)
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

func encodeTags(tags []model.KeyValue) ([]byte, error) {
	internalTags := make([]InternalTag, len(tags))

	for i := 0; i < len(tags); i++ {
		tag := tags[i]
		key, val := extractFromKeyValue(tag)
		t := InternalTag{
			Key:   key,
			Value: val,
		}
		internalTags[i] = t
	}

	encodedTags, err := json.Marshal(internalTags)
	if err != nil {
		log.Println("[error][encodetags] cannot marshal tags to json", encodedTags)
		return nil, err
	}

	return encodedTags, nil
}
