package domain

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrDuplicateName = errors.New("duplicate name")
	ErrInUse         = errors.New("in use")
	ErrInvalidInput  = errors.New("invalid input")
	ErrLookupFailed  = errors.New("lookup failed")
	ErrInvalidDay    = errors.New("invalid day")
	ErrSlotUnknown   = errors.New("slot unknown")
	ErrInvalidMacros = errors.New("invalid macros")
)
