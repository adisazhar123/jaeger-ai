package main

import (
	"context"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jmoiron/sqlx"
)

type ReaderDbClient struct {
	db *sqlx.DB
}

func (r ReaderDbClient) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	//TODO implement me
	panic("implement me")
}

func (r ReaderDbClient) GetServices(ctx context.Context) ([]string, error) {
	query := "SELECT name FROM services WHERE deleted_at IS NULL"
	rows, err := r.db.NamedQueryContext(ctx, query, struct{}{})
	if err != nil {
		return nil, err
	}
	services := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		services = append(services, name)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return services, nil
}

func (r ReaderDbClient) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	selectQuery := "SELECT o.name, o.kind FROM operations o INNER JOIN services s ON o.service_id = s.id WHERE (o.kind = :kind OR :kind = '') AND s.name = :name AND o.deleted_at IS NULL"

	rows, err := r.db.NamedQueryContext(ctx, selectQuery, struct {
		Kind string `db:"kind"`
		Name string `db:"name"`
	}{
		Kind: query.SpanKind,
		Name: query.ServiceName,
	})

	if err != nil {
		return nil, err
	}

	operations := make([]spanstore.Operation, 0)
	for rows.Next() {
		s := struct {
			Name string `db:"name"`
			Kind string `db:"kind"`
		}{}
		if err := rows.StructScan(&s); err != nil {
			return nil, err
		}
		operations = append(operations, spanstore.Operation{
			Name:     s.Name,
			SpanKind: s.Kind,
		})
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return operations, nil
}

func (r ReaderDbClient) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	//TODO implement me
	panic("implement me")
}

func (r ReaderDbClient) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	//TODO implement me
	panic("implement me")
}
