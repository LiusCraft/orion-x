package platformresource

import "errors"

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrConflict        = errors.New("conflict")
	ErrNotFound        = errors.New("resource not found")
)
