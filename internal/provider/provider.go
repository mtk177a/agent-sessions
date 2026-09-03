package provider

import (
	"context"
	"errors"
	"sort"

	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
)

var ErrNotFound = errors.New("source not found")

type SourceResult struct {
	Status    contract.Status
	Sources   []contract.Source
	Omissions []contract.Omission
	Err       error
}

type EventResult struct {
	Status    contract.Status
	Events    []contract.Event
	Omissions []contract.Omission
	Err       error
}

type EvidenceResult struct {
	Status    contract.Status
	Chunks    []contract.EvidenceChunk
	Omissions []contract.Omission
	Err       error
}

type Adapter interface {
	config.Defaults
	Name() string
	List(context.Context, config.Source) SourceResult
	Show(context.Context, config.Source, string) SourceResult
	Events(context.Context, config.Source, string) EventResult
	Evidence(context.Context, config.Source, string) EvidenceResult
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	registry := &Registry{adapters: map[string]Adapter{}}
	for _, adapter := range adapters {
		registry.adapters[adapter.Name()] = adapter
	}
	return registry
}

func (r *Registry) Get(name string) (Adapter, bool) {
	if r == nil {
		return nil, false
	}
	adapter, ok := r.adapters[name]
	return adapter, ok
}

func (r *Registry) All() []Adapter {
	if r == nil {
		return nil
	}
	result := make([]Adapter, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		result = append(result, adapter)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name() < result[j].Name() })
	return result
}
