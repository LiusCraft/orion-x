package store

import "errors"

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("store: record not found")

// ErrSystemRecord is returned when attempting to mutate a system-managed record.
var ErrSystemRecord = errors.New("store: cannot modify system record")
