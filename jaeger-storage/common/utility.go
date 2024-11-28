package common

import (
	"encoding/base64"
	"encoding/json"
	"github.com/jaegertracing/jaeger/model"
	"log"
	"os"
)

// adapted from https://stackoverflow.com/questions/15334220/encode-decode-base64
func Base64Encode(in []byte) string {
	return base64.StdEncoding.EncodeToString(in)
}

// adapted from https://stackoverflow.com/questions/15334220/encode-decode-base64
func Base64Decode(str string) ([]byte, bool) {
	data, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return []byte{}, true
	}
	return data, false
}

func EncodeKeyValue(kv model.KeyValue) *InternalKeyValue {
	return &InternalKeyValue{
		Key:       kv.Key,
		Value:     kv.Value(),
		ValueType: kv.VType,
	}
}

func EncodeLogs(logs []model.Log) ([]byte, []InternalLog, error) {
	internalLogs := make([]InternalLog, len(logs))

	for i := 0; i < len(logs); i++ {
		l := logs[i]
		fields := make([]InternalKeyValue, len(l.Fields))
		for j := 0; j < len(l.Fields); j++ {
			field := l.Fields[j]
			newKv := EncodeKeyValue(field)
			fields[j] = *newKv
		}

		internalLog := InternalLog{
			Timestamp: l.GetTimestamp(),
			Fields:    fields,
		}
		internalLogs[i] = internalLog
	}

	//fmt.Printf("internal logs %+v\n", internalLogs)

	data, err := json.Marshal(internalLogs)
	return data, internalLogs, err
}

func DecodeLogs(data []byte) ([]model.Log, error) {
	var internalLogs []InternalLog
	if err := json.Unmarshal(data, &internalLogs); err != nil {
		return nil, err
	}
	decodedLogs := make([]model.Log, len(internalLogs))
	for i := 0; i < len(internalLogs); i++ {
		internalLog := internalLogs[i]

		decodedFields := make([]model.KeyValue, 0)
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

func EncodeReferences(references []model.SpanRef) ([]byte, []InternalSpanRef, error) {
	internalReferences := make([]InternalSpanRef, len(references))
	for i := 0; i < len(references); i++ {
		ref := references[i]
		internalRef := InternalSpanRef{
			TraceId: ref.TraceID.String(),
			SpanId:  ref.SpanID.String(),
			RefType: uint64(ref.RefType),
		}
		internalReferences[i] = internalRef
	}

	result, err := json.Marshal(internalReferences)
	return result, internalReferences, err
}

func DecodeReferences(data []byte) ([]model.SpanRef, error) {
	var internalRefs []InternalSpanRef
	if err := json.Unmarshal(data, &internalRefs); err != nil {
		log.Println(err)
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

func EncodeTags(tags []model.KeyValue) ([]byte, error) {
	internalTags := make([]InternalKeyValue, len(tags))

	for i := 0; i < len(tags); i++ {
		tag := tags[i]
		t := EncodeKeyValue(tag)
		internalTags[i] = *t
	}

	encodedTags, err := json.Marshal(internalTags)
	if err != nil {
		log.Println("[error][encodetags] cannot marshal tags to json", encodedTags)
		return nil, err
	}

	return encodedTags, nil
}

func DecodeTags(data []byte) ([]model.KeyValue, error) {
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

func WriteToFile(filename string, content string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	defer f.Close()

	if _, err = f.WriteString(content); err != nil {
		return err
	}

	return nil
}
