package main

import (
	"context"
	"github.com/jaegertracing/jaeger/model"
	"time"
)

type dependencyReaderStub struct{}

// TODO: implement
func (d dependencyReaderStub) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return []model.DependencyLink{}, nil
}
