package toolruntime

import "errors"

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("tool runtime resource not found")
	ErrBusinessRule    = errors.New("business rule violated")
)
