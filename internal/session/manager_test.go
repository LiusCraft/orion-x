package session

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/pkg/pipeline"
)

type stoppingPipeline struct{ stopped bool }

func (p *stoppingPipeline) Start(context.Context) error     { return nil }
func (p *stoppingPipeline) Stop() error                     { p.stopped = true; return nil }
func (p *stoppingPipeline) Interrupt() error                { return nil }
func (p *stoppingPipeline) Output() <-chan pipeline.Message { return nil }
func (p *stoppingPipeline) Input() chan<- pipeline.Message  { return nil }
func (p *stoppingPipeline) Emit(pipeline.Message) error     { return nil }

func TestManagerCloseSessionStopsPipeline(t *testing.T) {
	mgr := NewManager()
	pl := &stoppingPipeline{}
	sess := New(SessionMeta{})
	sess.Pipeline = pl
	mgr.Add(sess)
	mgr.CloseSession(sess.ID)
	if !pl.stopped {
		t.Fatal("pipeline was not stopped")
	}
	if _, ok := mgr.Get(sess.ID); ok {
		t.Fatal("closed session remains in manager")
	}
}
