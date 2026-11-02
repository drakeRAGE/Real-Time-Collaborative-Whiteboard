package service

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrNotMember         = errors.New("not a member")
	ErrUnauthenticated   = errors.New("unauthenticated")
	ErrInvalidArgument   = errors.New("invalid argument")
	ErrRateLimited       = errors.New("rate limited")
)
