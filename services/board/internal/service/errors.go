package service

import "errors"

// Sentinel errors mapped to gRPC codes in the transport layer.
var (
	ErrNotFound        = errors.New("not found")
	ErrNotAdmin        = errors.New("not admin")
	ErrNotMember       = errors.New("not a member")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNothingToUndo   = errors.New("nothing to undo")
	ErrRedoRejected    = errors.New("redo rejected")
)

// IsPermanent reports errors that must be acked and not retried.
func IsPermanent(err error) bool {
	return errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrNotAdmin) ||
		errors.Is(err, ErrNotMember) ||
		errors.Is(err, ErrUnauthenticated) ||
		errors.Is(err, ErrInvalidArgument) ||
		errors.Is(err, ErrNothingToUndo) ||
		errors.Is(err, ErrRedoRejected)
}
