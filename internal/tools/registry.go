package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/liuscraft/orion-x/internal/llm"
)

type Spec struct {
	Name         string
	Description  string
	Parameters   map[string]any
	ParallelSafe bool
	Execute      func(ctx context.Context, arguments json.RawMessage) (Result, error)
}

type Result struct {
	Output string
	Error  error
}

type Registry struct {
	specs map[string]Spec
	order []string
}

func NewRegistry(specs ...Spec) *Registry {
	r := &Registry{
		specs: make(map[string]Spec),
		order: make([]string, 0, len(specs)),
	}
	for _, spec := range specs {
		r.specs[spec.Name] = spec
		r.order = append(r.order, spec.Name)
	}
	return r
}

func (r *Registry) Add(spec Spec) {
	r.specs[spec.Name] = spec
	r.order = append(r.order, spec.Name)
}

func (r *Registry) Definitions() []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		spec := r.specs[name]
		defs = append(defs, llm.ToolDefinition{
			Name:        spec.Name,
			Description: spec.Description,
			Parameters:  spec.Parameters,
		})
	}
	return defs
}

func (r *Registry) CanExecute(name string) bool {
	_, ok := r.specs[name]
	return ok
}

func (r *Registry) Execute(ctx context.Context, name string, arguments json.RawMessage) (Result, error) {
	spec, ok := r.specs[name]
	if !ok {
		return Result{}, fmt.Errorf("unknown tool: %s", name)
	}
	return spec.Execute(ctx, arguments)
}
