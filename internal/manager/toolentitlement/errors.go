package toolentitlement

import "errors"

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrConflict        = errors.New("conflict")
	ErrNotFound        = errors.New("entitlement not found")
	ErrBusinessRule    = errors.New("business rule violated")
)
