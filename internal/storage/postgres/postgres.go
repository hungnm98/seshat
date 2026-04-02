package postgres

import "errors"

var ErrNotImplemented = errors.New("postgres storage is not implemented in the MVP skeleton; use memory store for now")
