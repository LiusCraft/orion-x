package storage

import "testing"

type tableNamer interface {
	TableName() string
}

func TestMigrationTableNames(t *testing.T) {
	expected := map[string]struct{}{
		"users":              {},
		"platform_resources": {},
		"voicebots":          {},
		"devices":            {},
		"device_bindings":    {},
	}

	got := migrationTableNames()
	if len(got) != len(expected) {
		t.Fatalf("expected %d tables, got %d", len(expected), len(got))
	}
	for _, name := range got {
		if _, ok := expected[name]; !ok {
			t.Fatalf("unexpected migration table: %s", name)
		}
	}
}

func TestMigrationModels_CoversCoreTables(t *testing.T) {
	expected := map[string]struct{}{
		"users":              {},
		"platform_resources": {},
		"voicebots":          {},
		"devices":            {},
		"device_bindings":    {},
	}

	for _, model := range MigrationModels() {
		namer, ok := model.(tableNamer)
		if !ok {
			t.Fatalf("model does not implement TableName(): %T", model)
		}
		delete(expected, namer.TableName())
	}

	if len(expected) != 0 {
		t.Fatalf("missing models for tables: %#v", expected)
	}
}
