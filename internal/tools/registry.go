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
	if _, exists := r.specs[spec.Name]; !exists {
		r.order = append(r.order, spec.Name)
	}
	r.specs[spec.Name] = spec
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

// IsParallelSafe 返回该工具是否可与同轮其它工具并发执行。未注册的工具返回 false。
func (r *Registry) IsParallelSafe(name string) bool {
	spec, ok := r.specs[name]
	return ok && spec.ParallelSafe
}

func (r *Registry) Execute(ctx context.Context, name string, arguments json.RawMessage) (Result, error) {
	spec, ok := r.specs[name]
	if !ok {
		return Result{}, fmt.Errorf("unknown tool: %s", name)
	}
	return spec.Execute(ctx, arguments)
}

// Specs returns all registered specs in registration order.
func (r *Registry) Specs() []Spec {
	out := make([]Spec, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.specs[name])
	}
	return out
}

// Clone returns a new Registry with the same specs, independent of the original.
func (r *Registry) Clone() *Registry {
	c := &Registry{
		specs: make(map[string]Spec, len(r.specs)),
		order: make([]string, len(r.order)),
	}
	copy(c.order, r.order)
	for k, v := range r.specs {
		c.specs[k] = v
	}
	return c
}
