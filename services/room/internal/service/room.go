package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/publish"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/repo"
)

// Service is the room authorization authority.
// Every method reads the caller from gRPC metadata. CheckMembership is the
// only method that also takes a user id, and that id is the lookup subject.
type Service struct {
	store repo.Store
	pub   publish.Publisher
}

// New returns a service that uses store and publishes lifecycle events on pub.
func New(store repo.Store, pub publish.Publisher) *Service {
	return &Service{store: store, pub: pub}
}

// RoomView is a live room returned to callers.
type RoomView struct {
	ID        uuid.UUID
	AdminID   uuid.UUID
	Title     string
	CreatedAt time.Time
}

// LeaveResult describes admin transfer or soft-delete after a leave.
type LeaveResult struct {
	NewAdminID uuid.UUID
	Deleted    bool
}

// InviteView is a newly created invite. Token is plaintext and is not stored.
type InviteView struct {
	ID        uuid.UUID
	Token     string
	ExpiresAt time.Time
}

// CreateRoom inserts the room and an admin membership in one transaction.
func (s *Service) CreateRoom(ctx context.Context, title string) (RoomView, error) {
	caller, err := principal(ctx)
	if err != nil {
		return RoomView{}, err
	}
	var view RoomView
	err = s.store.WithTx(ctx, func(q repo.Tx) error {
		room, err := q.CreateRoom(ctx, caller, title)
		if err != nil {
			return err
		}
		if _, err := q.UpsertMember(ctx, room.ID, caller, roleAdmin); err != nil {
			return err
		}
		if err := q.InsertAudit(ctx, caller, "room.create", room.ID.String(), room.ID, []byte(`{}`)); err != nil {
			return err
		}
		view = toView(room)
		view.AdminID = caller
		return nil
	})
	return view, err
}

// GetRoom returns a room the caller belongs to.
func (s *Service) GetRoom(ctx context.Context, roomID uuid.UUID) (RoomView, error) {
	caller, err := principal(ctx)
	if err != nil {
		return RoomView{}, err
	}
	var view RoomView
	err = s.store.WithTx(ctx, func(q repo.Tx) error {
		room, err := liveRoom(ctx, q, roomID)
		if err != nil {
			return err
		}
		if _, err := requireMember(ctx, q, roomID, caller); err != nil {
			return err
		}
		view = toView(room)
		return nil
	})
	return view, err
}

// JoinRoom adds the caller. A token, when present, must match a live invite
// for this room; its hash, expiry, and max_uses are checked in the same transaction.
func (s *Service) JoinRoom(ctx context.Context, roomID uuid.UUID, token string) (RoomView, string, error) {
	caller, err := principal(ctx)
	if err != nil {
		return RoomView{}, "", err
	}
	var (
		view   RoomView
		role   string
		member repo.Member
		fresh  bool
	)
	err = s.store.WithTx(ctx, func(q repo.Tx) error {
		room, err := liveRoom(ctx, q, roomID)
		if err != nil {
			return err
		}
		existing, err := q.GetMemberRole(ctx, roomID, caller)
		if err != nil && !errors.Is(err, repo.ErrNotFound) {
			return err
		}
		if existing == roleAdmin || existing == roleMember {
			member, err = q.GetMember(ctx, roomID, caller)
			if err != nil {
				return err
			}
			view = toView(room)
			role = existing
			return nil
		}
		if token != "" {
			if err := consumeInvite(ctx, q, roomID, token); err != nil {
				return err
			}
		}
		if _, err := q.UpsertMember(ctx, roomID, caller, roleMember); err != nil {
			return err
		}
		member, err = q.GetMember(ctx, roomID, caller)
		if err != nil {
			return err
		}
		if err := q.InsertAudit(ctx, caller, "room.join", roomID.String(), roomID, []byte(`{}`)); err != nil {
			return err
		}
		view = toView(room)
		role = member.Role
		fresh = true
		return nil
	})
	if err != nil {
		return RoomView{}, "", err
	}
	if fresh {
		if err := s.publishMember(ctx, subjectJoined, roomID.String(), member); err != nil {
			return RoomView{}, "", err
		}
	}
	return view, role, nil
}

// LeaveRoom removes the caller. An admin leave transfers to the oldest other
// member, or soft-deletes the room when nobody remains.
func (s *Service) LeaveRoom(ctx context.Context, roomID uuid.UUID) (LeaveResult, error) {
	caller, err := principal(ctx)
	if err != nil {
		return LeaveResult{}, err
	}
	var (
		res    LeaveResult
		leaver repo.Member
	)
	err = s.store.WithTx(ctx, func(q repo.Tx) error {
		role, err := q.GetMemberRole(ctx, roomID, caller)
		if errors.Is(err, repo.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		leaver, err = q.GetMember(ctx, roomID, caller)
		if err != nil {
			return err
		}
		members, err := q.ListMembers(ctx, roomID)
		if err != nil {
			return err
		}
		if role == roleAdmin {
			next, ok := oldestOther(members, caller)
			if !ok {
				if err := q.DeleteMember(ctx, roomID, caller); err != nil {
					return err
				}
				if err := q.SoftDeleteRoom(ctx, roomID); err != nil {
					return err
				}
				res.Deleted = true
			} else {
				if err := q.TransferAdmin(ctx, roomID, next); err != nil {
					return err
				}
				if err := q.DeleteMember(ctx, roomID, caller); err != nil {
					return err
				}
				res.NewAdminID = next
			}
		} else {
			if err := q.DeleteMember(ctx, roomID, caller); err != nil {
				return err
			}
			if len(members) <= 1 {
				if err := q.SoftDeleteRoom(ctx, roomID); err != nil {
					return err
				}
				res.Deleted = true
			}
		}
		meta, err := json.Marshal(leaveMeta(res))
		if err != nil {
			return err
		}
		return q.InsertAudit(ctx, caller, "room.leave", roomID.String(), roomID, meta)
	})
	if err != nil {
		return LeaveResult{}, err
	}
	if err := s.publishMember(ctx, subjectLeft, roomID.String(), leaver); err != nil {
		return LeaveResult{}, err
	}
	if res.Deleted {
		if err := s.publishDeleted(ctx, roomID.String()); err != nil {
			return LeaveResult{}, err
		}
	}
	return res, nil
}

// DeleteRoom soft-deletes a room. The caller must be the admin.
func (s *Service) DeleteRoom(ctx context.Context, roomID uuid.UUID) error {
	caller, err := principal(ctx)
	if err != nil {
		return err
	}
	err = s.store.WithTx(ctx, func(q repo.Tx) error {
		if _, err := liveRoom(ctx, q, roomID); err != nil {
			return err
		}
		role, err := q.GetMemberRole(ctx, roomID, caller)
		if err != nil && !errors.Is(err, repo.ErrNotFound) {
			return err
		}
		if role != roleAdmin {
			return ErrNotAdmin
		}
		if err := q.SoftDeleteRoom(ctx, roomID); err != nil {
			return err
		}
		return q.InsertAudit(ctx, caller, "room.delete", roomID.String(), roomID, []byte(`{}`))
	})
	if err != nil {
		return err
	}
	return s.publishDeleted(ctx, roomID.String())
}

// ListMembers returns members when the caller is one of them.
func (s *Service) ListMembers(ctx context.Context, roomID uuid.UUID) ([]repo.Member, error) {
	caller, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	var members []repo.Member
	err = s.store.WithTx(ctx, func(q repo.Tx) error {
		if _, err := liveRoom(ctx, q, roomID); err != nil {
			return err
		}
		if _, err := requireMember(ctx, q, roomID, caller); err != nil {
			return err
		}
		var err error
		members, err = q.ListMembers(ctx, roomID)
		return err
	})
	return members, err
}

// TransferAdmin moves admin from the caller to newAdmin. newAdmin is the
// target member, not the caller.
func (s *Service) TransferAdmin(ctx context.Context, roomID, newAdmin uuid.UUID) error {
	caller, err := principal(ctx)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q repo.Tx) error {
		if _, err := liveRoom(ctx, q, roomID); err != nil {
			return err
		}
		role, err := q.GetMemberRole(ctx, roomID, caller)
		if err != nil && !errors.Is(err, repo.ErrNotFound) {
			return err
		}
		if role != roleAdmin {
			return ErrNotAdmin
		}
		target, err := q.GetMemberRole(ctx, roomID, newAdmin)
		if errors.Is(err, repo.ErrNotFound) || target == "" {
			return ErrNotMember
		}
		if err != nil {
			return err
		}
		if newAdmin == caller {
			return nil
		}
		if err := q.TransferAdmin(ctx, roomID, newAdmin); err != nil {
			return err
		}
		meta, err := json.Marshal(map[string]string{"new_admin_id": newAdmin.String()})
		if err != nil {
			return err
		}
		return q.InsertAudit(ctx, caller, "room.transfer_admin", newAdmin.String(), roomID, meta)
	})
}

// CreateInvite stores the SHA-256 of a new token. The plaintext is returned once.
func (s *Service) CreateInvite(ctx context.Context, roomID uuid.UUID, expiresIn time.Duration, maxUses int32) (InviteView, error) {
	caller, err := principal(ctx)
	if err != nil {
		return InviteView{}, err
	}
	token, hash, err := newToken()
	if err != nil {
		return InviteView{}, err
	}
	expires := time.Now().Add(expiresIn).UTC()
	var view InviteView
	err = s.store.WithTx(ctx, func(q repo.Tx) error {
		if _, err := liveRoom(ctx, q, roomID); err != nil {
			return err
		}
		role, err := q.GetMemberRole(ctx, roomID, caller)
		if err != nil && !errors.Is(err, repo.ErrNotFound) {
			return err
		}
		if role != roleAdmin {
			return ErrNotAdmin
		}
		inv, err := q.CreateInvite(ctx, roomID, hash, expires, maxUses)
		if err != nil {
			return err
		}
		meta, err := json.Marshal(map[string]string{"invite_id": inv.ID.String()})
		if err != nil {
			return err
		}
		if err := q.InsertAudit(ctx, caller, "room.invite", inv.ID.String(), roomID, meta); err != nil {
			return err
		}
		view = InviteView{ID: inv.ID, Token: token, ExpiresAt: inv.ExpiresAt}
		return nil
	})
	return view, err
}

// ValidateInvite reports whether token is currently usable. It does not consume a use.
func (s *Service) ValidateInvite(ctx context.Context, token string) (bool, uuid.UUID, error) {
	if _, err := principal(ctx); err != nil {
		return false, uuid.Nil, err
	}
	sum := sha256.Sum256([]byte(token))
	var (
		valid  bool
		roomID uuid.UUID
	)
	err := s.store.WithTx(ctx, func(q repo.Tx) error {
		inv, err := q.GetInviteByHash(ctx, sum[:])
		if errors.Is(err, repo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !inv.ExpiresAt.After(time.Now()) || inv.Uses >= inv.MaxUses {
			return nil
		}
		room, err := q.GetRoom(ctx, inv.RoomID)
		if errors.Is(err, repo.ErrNotFound) || room.Deleted {
			return nil
		}
		if err != nil {
			return err
		}
		valid = true
		roomID = inv.RoomID
		return nil
	})
	return valid, roomID, err
}

// CheckMembership returns the role of userID. userID is the subject the
// gateway asked about. The caller still has to be authenticated.
// The lookup is GetMemberRole, one indexed query.
func (s *Service) CheckMembership(ctx context.Context, roomID, userID uuid.UUID) (string, error) {
	if _, err := principal(ctx); err != nil {
		return "", err
	}
	role, err := s.store.GetMemberRole(ctx, roomID, userID)
	if errors.Is(err, repo.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return role, nil
}

func liveRoom(ctx context.Context, q repo.Tx, roomID uuid.UUID) (repo.Room, error) {
	room, err := q.GetRoom(ctx, roomID)
	if errors.Is(err, repo.ErrNotFound) || room.Deleted {
		return repo.Room{}, ErrNotFound
	}
	if err != nil {
		return repo.Room{}, err
	}
	return room, nil
}

func requireMember(ctx context.Context, q repo.Tx, roomID, userID uuid.UUID) (string, error) {
	role, err := q.GetMemberRole(ctx, roomID, userID)
	if errors.Is(err, repo.ErrNotFound) || role == "" {
		return "", ErrNotMember
	}
	if err != nil {
		return "", err
	}
	return role, nil
}

func consumeInvite(ctx context.Context, q repo.Tx, roomID uuid.UUID, token string) error {
	sum := sha256.Sum256([]byte(token))
	inv, err := q.GetInviteByHash(ctx, sum[:])
	if errors.Is(err, repo.ErrNotFound) {
		return ErrBadInvite
	}
	if err != nil {
		return err
	}
	if inv.RoomID != roomID {
		return ErrBadInvite
	}
	if err := q.IncrementInviteUse(ctx, inv.ID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrBadInvite
		}
		return err
	}
	return nil
}

func oldestOther(members []repo.Member, self uuid.UUID) (uuid.UUID, bool) {
	for _, m := range members {
		if m.UserID != self {
			return m.UserID, true
		}
	}
	return uuid.Nil, false
}

func leaveMeta(res LeaveResult) map[string]any {
	meta := map[string]any{"room_deleted": res.Deleted}
	if res.NewAdminID != uuid.Nil {
		meta["new_admin_id"] = res.NewAdminID.String()
	}
	return meta
}

func toView(room repo.Room) RoomView {
	return RoomView{ID: room.ID, AdminID: room.AdminID, Title: room.Title, CreatedAt: room.CreatedAt}
}

func newToken() (string, []byte, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("invite token: %w", err)
	}
	plain := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(plain))
	return plain, sum[:], nil
}
