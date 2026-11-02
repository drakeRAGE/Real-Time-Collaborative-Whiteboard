package service

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/repo"
)

// mem is an in-memory repo.Store for unit tests.
type mem struct {
	rooms    map[uuid.UUID]repo.Room
	members  map[uuid.UUID][]repo.Member
	invites  []repo.Invite
	profiles map[uuid.UUID]string
	audits   []string
}

func newMem() *mem {
	return &mem{
		rooms:    map[uuid.UUID]repo.Room{},
		members:  map[uuid.UUID][]repo.Member{},
		profiles: map[uuid.UUID]string{},
	}
}

func (m *mem) WithTx(_ context.Context, fn func(repo.Tx) error) error { return fn(m) }

func (m *mem) CreateRoom(_ context.Context, adminID uuid.UUID, title string) (repo.Room, error) {
	room := repo.Room{ID: uuid.New(), AdminID: adminID, Title: title, CreatedAt: time.Now().UTC()}
	m.rooms[room.ID] = room
	return room, nil
}

func (m *mem) GetRoom(_ context.Context, id uuid.UUID) (repo.Room, error) {
	room, ok := m.rooms[id]
	if !ok {
		return repo.Room{}, repo.ErrNotFound
	}
	return room, nil
}

func (m *mem) SoftDeleteRoom(_ context.Context, id uuid.UUID) error {
	room, ok := m.rooms[id]
	if !ok || room.Deleted {
		return repo.ErrNotFound
	}
	room.Deleted = true
	room.AdminID = uuid.Nil
	m.rooms[id] = room
	return nil
}

func (m *mem) UpsertMember(_ context.Context, roomID, userID uuid.UUID, role string) (repo.Member, error) {
	for _, member := range m.members[roomID] {
		if member.UserID == userID {
			return member, nil
		}
	}
	member := repo.Member{UserID: userID, Username: m.name(userID), Role: role, JoinedAt: time.Now().UTC()}
	m.members[roomID] = append(m.members[roomID], member)
	return member, nil
}

func (m *mem) DeleteMember(_ context.Context, roomID, userID uuid.UUID) error {
	kept := make([]repo.Member, 0, len(m.members[roomID]))
	for _, member := range m.members[roomID] {
		if member.UserID != userID {
			kept = append(kept, member)
		}
	}
	m.members[roomID] = kept
	return nil
}

func (m *mem) ListMembers(_ context.Context, roomID uuid.UUID) ([]repo.Member, error) {
	out := append([]repo.Member(nil), m.members[roomID]...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].JoinedAt.Equal(out[j].JoinedAt) {
			return bytes.Compare(out[i].UserID[:], out[j].UserID[:]) < 0
		}
		return out[i].JoinedAt.Before(out[j].JoinedAt)
	})
	return out, nil
}

func (m *mem) GetMember(ctx context.Context, roomID, userID uuid.UUID) (repo.Member, error) {
	members, err := m.ListMembers(ctx, roomID)
	if err != nil {
		return repo.Member{}, err
	}
	for _, member := range members {
		if member.UserID == userID {
			member.Username = m.name(userID)
			return member, nil
		}
	}
	return repo.Member{}, repo.ErrNotFound
}

func (m *mem) GetMemberRole(_ context.Context, roomID, userID uuid.UUID) (string, error) {
	room, ok := m.rooms[roomID]
	if !ok || room.Deleted {
		return "", repo.ErrNotFound
	}
	for _, member := range m.members[roomID] {
		if member.UserID == userID {
			return member.Role, nil
		}
	}
	return "", repo.ErrNotFound
}

func (m *mem) TransferAdmin(_ context.Context, roomID, newAdmin uuid.UUID) error {
	room, ok := m.rooms[roomID]
	if !ok || room.Deleted {
		return repo.ErrNotFound
	}
	found := false
	for i, member := range m.members[roomID] {
		switch {
		case member.UserID == newAdmin:
			m.members[roomID][i].Role = roleAdmin
			found = true
		case member.Role == roleAdmin:
			m.members[roomID][i].Role = roleMember
		}
	}
	if !found {
		return repo.ErrNotFound
	}
	room.AdminID = newAdmin
	m.rooms[roomID] = room
	return nil
}

func (m *mem) CreateInvite(_ context.Context, roomID uuid.UUID, hash []byte, expiresAt time.Time, maxUses int32) (repo.Invite, error) {
	inv := repo.Invite{
		ID:        uuid.New(),
		RoomID:    roomID,
		TokenHash: append([]byte(nil), hash...),
		ExpiresAt: expiresAt,
		MaxUses:   maxUses,
	}
	m.invites = append(m.invites, inv)
	return inv, nil
}

func (m *mem) GetInviteByHash(_ context.Context, hash []byte) (repo.Invite, error) {
	for _, inv := range m.invites {
		if bytes.Equal(inv.TokenHash, hash) {
			inv.TokenHash = append([]byte(nil), inv.TokenHash...)
			return inv, nil
		}
	}
	return repo.Invite{}, repo.ErrNotFound
}

func (m *mem) IncrementInviteUse(_ context.Context, id uuid.UUID) error {
	for i, inv := range m.invites {
		if inv.ID != id {
			continue
		}
		if !inv.ExpiresAt.After(time.Now()) || inv.Uses >= inv.MaxUses {
			return repo.ErrNotFound
		}
		m.invites[i].Uses++
		return nil
	}
	return repo.ErrNotFound
}

func (m *mem) InsertAudit(_ context.Context, _ uuid.UUID, action, _ string, _ uuid.UUID, _ []byte) error {
	m.audits = append(m.audits, action)
	return nil
}

func (m *mem) name(id uuid.UUID) string {
	if n := m.profiles[id]; n != "" {
		return n
	}
	return "user"
}

func (m *mem) addRoom(admin uuid.UUID, title string) repo.Room {
	room, err := m.CreateRoom(context.Background(), admin, title)
	if err != nil {
		panic(err)
	}
	return room
}

func (m *mem) addMember(room, user uuid.UUID, role string, joined time.Time, username string) {
	m.profiles[user] = username
	m.members[room] = append(m.members[room], repo.Member{
		UserID: user, Username: username, Role: role, JoinedAt: joined,
	})
}

func (m *mem) addInvite(room uuid.UUID, hash []byte, expires time.Time, maxUses, uses int32) {
	m.invites = append(m.invites, repo.Invite{
		ID: uuid.New(), RoomID: room, TokenHash: append([]byte(nil), hash...),
		ExpiresAt: expires, MaxUses: maxUses, Uses: uses,
	})
}

func (m *mem) inviteUses(room uuid.UUID) int32 {
	for _, inv := range m.invites {
		if inv.RoomID == room {
			return inv.Uses
		}
	}
	return -1
}
