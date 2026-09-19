package repository

import "errors"

// 仓储层哨兵错误。
var (
	ErrNotFound       = errors.New("not found")
	ErrDuplicate      = errors.New("duplicate record")
	ErrStatusConflict = errors.New("incident status transition conflict")
)
