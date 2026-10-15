package storage

import "errors"

// ErrNotFound is returned when an entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a unique constraint is violated.
var ErrConflict = errors.New("conflict")
