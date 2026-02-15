package app

import (
	"context"
	"errors"
	"testing"

	"github.com/liuscraft/orion-x/internal/manager/storage"
)

type fakeMigrator struct {
	calls int
	err   error
	res   storage.MigrationResult
}

func (m *fakeMigrator) Migrate(context.Context) (storage.MigrationResult, error) {
	m.calls++
	if m.err != nil {
		return storage.MigrationResult{}, m.err
	}
	return m.res, nil
}

type fakeServer struct {
	startCalls    int
	shutdownCalls int
	startErr      error
}

func (s *fakeServer) Start() error {
	s.startCalls++
	return s.startErr
}

func (s *fakeServer) Shutdown(context.Context) error {
	s.shutdownCalls++
	return nil
}

func TestLifecycleBootstrap_MigrateOnly(t *testing.T) {
	m := &fakeMigrator{res: storage.MigrationResult{TargetVersion: 1, Applied: true}}
	s := &fakeServer{}
	lifecycle := NewLifecycle(true, m, s)

	_, err := lifecycle.Bootstrap(context.Background(), true)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("expected migrate call, got %d", m.calls)
	}
	if s.startCalls != 0 {
		t.Fatalf("server should not start in migrate-only mode")
	}
}

func TestLifecycleBootstrap_StartupWithoutAutoMigrate(t *testing.T) {
	m := &fakeMigrator{}
	s := &fakeServer{}
	lifecycle := NewLifecycle(false, m, s)

	_, err := lifecycle.Bootstrap(context.Background(), false)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if m.calls != 0 {
		t.Fatalf("expected no migrate call, got %d", m.calls)
	}
	if s.startCalls != 0 {
		t.Fatalf("server should not start during bootstrap")
	}
}

func TestLifecycleStart(t *testing.T) {
	m := &fakeMigrator{}
	s := &fakeServer{}
	lifecycle := NewLifecycle(false, m, s)

	err := lifecycle.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if s.startCalls != 1 {
		t.Fatalf("expected server start call, got %d", s.startCalls)
	}
}

func TestLifecycleBootstrap_MigrateFailure(t *testing.T) {
	m := &fakeMigrator{err: errors.New("boom")}
	s := &fakeServer{}
	lifecycle := NewLifecycle(true, m, s)

	_, err := lifecycle.Bootstrap(context.Background(), false)
	if err == nil {
		t.Fatalf("expected migration error")
	}
	if s.startCalls != 0 {
		t.Fatalf("server should not start after migrate failure")
	}
}
