package service

import "errors"

// Sentinel errors mapped to gRPC codes in the transport layer.
var (
	ErrNotFound        = errors.New("not found")
	ErrNotAdmin        = errors.New("not admin")
	ErrNotMember       = errors.New("not a member")
	ErrBadInvite       = errors.New("bad invite")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrInvalidArgument = errors.New("invalid argument")
)

const (
	roleAdmin  = "admin"
	roleMember = "member"

	subjectJoined  = "wb.room.member_joined.v1"
	subjectLeft    = "wb.room.member_left.v1"
	subjectDeleted = "wb.room.deleted.v1"
)
