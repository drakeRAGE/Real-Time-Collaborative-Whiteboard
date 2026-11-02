package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/room/internal/repo"
)

type tape struct {
	subjects []string
	payloads [][]byte
	rooms    []string
}

func (t *tape) Publish(_ context.Context, subject string, data []byte, msgID, roomID string) error {
	if msgID == "" {
		return errors.New("msg id required")
	}
	t.subjects = append(t.subjects, subject)
	t.payloads = append(t.payloads, append([]byte(nil), data...))
	t.rooms = append(t.rooms, roomID)
	return nil
}

func asUser(id uuid.UUID) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(mdUserID, id.String()))
}

func TestCreateRoomMakesCreatorAdmin(t *testing.T) {
	st := newMem()
	pub := &tape{}
	svc := New(st, pub)
	admin := uuid.New()
	st.profiles[admin] = "ada"

	room, err := svc.CreateRoom(asUser(admin), "Studio")
	if err != nil {
		t.Fatal(err)
	}
	if room.AdminID != admin || room.Title != "Studio" {
		t.Fatalf("room %+v", room)
	}
	role, err := st.GetMemberRole(context.Background(), room.ID, admin)
	if err != nil || role != roleAdmin {
		t.Fatalf("role %s %v", role, err)
	}
	if len(pub.subjects) != 0 {
		t.Fatalf("create publishes nothing, got %v", pub.subjects)
	}
	if st.audits[0] != "room.create" {
		t.Fatalf("audit %v", st.audits)
	}
}

func TestJoinAndInvites(t *testing.T) {
	admin := uuid.New()
	guest := uuid.New()
	st := newMem()
	st.profiles[admin] = "ada"
	st.profiles[guest] = "bea"
	room := st.addRoom(admin, "Studio")
	st.addMember(room.ID, admin, roleAdmin, time.Now().Add(-time.Hour), "ada")
	svc := New(st, &tape{})

	t.Run("missing room", func(t *testing.T) {
		_, _, err := svc.JoinRoom(asUser(guest), uuid.New(), "")
		if !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	})

	t.Run("deleted room", func(t *testing.T) {
		if err := st.SoftDeleteRoom(context.Background(), room.ID); err != nil {
			t.Fatal(err)
		}
		_, _, err := svc.JoinRoom(asUser(guest), room.ID, "")
		if !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
		room = st.addRoom(admin, "Studio")
		st.addMember(room.ID, admin, roleAdmin, time.Now().Add(-time.Hour), "ada")
	})

	pub := &tape{}
	svc = New(st, pub)
	joined, role, err := svc.JoinRoom(asUser(guest), room.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if joined.ID != room.ID || role != roleMember {
		t.Fatalf("joined %+v %s", joined, role)
	}
	if pub.subjects[0] != subjectJoined || pub.rooms[0] != room.ID.String() {
		t.Fatalf("event %v %v", pub.subjects, pub.rooms)
	}
	var body roomv1.Member
	if err := proto.Unmarshal(pub.payloads[0], &body); err != nil {
		t.Fatal(err)
	}
	if body.GetUserId() != guest.String() || body.GetUsername() != "bea" || body.GetRole() != roomv1.Role_ROLE_MEMBER {
		t.Fatalf("member %+v", body.String())
	}

	before := len(pub.subjects)
	if _, _, err := svc.JoinRoom(asUser(guest), room.ID, "not-a-real-token"); err != nil {
		t.Fatal(err)
	}
	if len(pub.subjects) != before {
		t.Fatal("rejoin published")
	}

	other := st.addRoom(admin, "Other")
	st.addMember(other.ID, admin, roleAdmin, time.Now().Add(-time.Hour), "ada")
	otherToken := "other-room-token-0001"
	otherSum := sha256.Sum256([]byte(otherToken))
	st.addInvite(other.ID, otherSum[:], time.Now().Add(time.Hour), 2, 0)
	if _, _, err := svc.JoinRoom(asUser(uuid.New()), room.ID, otherToken); !errors.Is(err, ErrBadInvite) {
		t.Fatalf("wrong room: %v", err)
	}
	token := "good-token-0123456789"
	sum := sha256.Sum256([]byte(token))
	st.addInvite(room.ID, sum[:], time.Now().Add(time.Hour), 2, 0)
	newcomer := uuid.New()
	st.profiles[newcomer] = "cy"
	if _, role, err := svc.JoinRoom(asUser(newcomer), room.ID, token); err != nil || role != roleMember {
		t.Fatal(err, role)
	}
	if st.inviteUses(room.ID) != 1 {
		t.Fatalf("uses %d", st.inviteUses(room.ID))
	}

	expired := sha256.Sum256([]byte("expired-token-012345"))
	st.addInvite(room.ID, expired[:], time.Now().Add(-time.Minute), 2, 0)
	if _, _, err := svc.JoinRoom(asUser(uuid.New()), room.ID, "expired-token-012345"); !errors.Is(err, ErrBadInvite) {
		t.Fatalf("expired: %v", err)
	}
	spent := sha256.Sum256([]byte("spent-token-0123456"))
	st.addInvite(room.ID, spent[:], time.Now().Add(time.Hour), 1, 1)
	if _, _, err := svc.JoinRoom(asUser(uuid.New()), room.ID, "spent-token-0123456"); !errors.Is(err, ErrBadInvite) {
		t.Fatalf("spent: %v", err)
	}
	if _, _, err := svc.JoinRoom(asUser(uuid.New()), room.ID, "unknown-token-00000"); !errors.Is(err, ErrBadInvite) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestLeaveRoomAdminTransferAndDelete(t *testing.T) {
	t.Run("two members transfers to the other", func(t *testing.T) {
		admin, other := uuid.New(), uuid.New()
		st := newMem()
		room := st.addRoom(admin, "Studio")
		st.addMember(room.ID, admin, roleAdmin, time.Now().Add(-2*time.Hour), "ada")
		st.addMember(room.ID, other, roleMember, time.Now().Add(-time.Hour), "bea")
		pub := &tape{}
		res, err := New(st, pub).LeaveRoom(asUser(admin), room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if res.Deleted || res.NewAdminID != other {
			t.Fatalf("result %+v", res)
		}
		got, err := st.GetRoom(context.Background(), room.ID)
		if err != nil || got.Deleted || got.AdminID != other {
			t.Fatalf("room %+v %v", got, err)
		}
		role, err := st.GetMemberRole(context.Background(), room.ID, other)
		if err != nil || role != roleAdmin {
			t.Fatalf("new admin role %s %v", role, err)
		}
		if _, err := st.GetMemberRole(context.Background(), room.ID, admin); !errors.Is(err, repo.ErrNotFound) {
			t.Fatalf("leaver still present: %v", err)
		}
		if len(pub.subjects) != 1 || pub.subjects[0] != subjectLeft {
			t.Fatalf("events %v", pub.subjects)
		}
	})

	t.Run("one member soft-deletes the room", func(t *testing.T) {
		admin := uuid.New()
		st := newMem()
		room := st.addRoom(admin, "Studio")
		st.addMember(room.ID, admin, roleAdmin, time.Now(), "ada")
		pub := &tape{}
		res, err := New(st, pub).LeaveRoom(asUser(admin), room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Deleted || res.NewAdminID != uuid.Nil {
			t.Fatalf("result %+v", res)
		}
		got, err := st.GetRoom(context.Background(), room.ID)
		if err != nil || !got.Deleted {
			t.Fatalf("room %+v %v", got, err)
		}
		if pub.subjects[0] != subjectLeft || pub.subjects[1] != subjectDeleted {
			t.Fatalf("events %v", pub.subjects)
		}
		if string(pub.payloads[1]) != room.ID.String() {
			t.Fatalf("deleted payload %q", pub.payloads[1])
		}
	})

	t.Run("three members transfers to the oldest other", func(t *testing.T) {
		admin, older, newer := uuid.New(), uuid.New(), uuid.New()
		st := newMem()
		room := st.addRoom(admin, "Studio")
		now := time.Now()
		st.addMember(room.ID, older, roleMember, now.Add(-3*time.Hour), "older")
		st.addMember(room.ID, admin, roleAdmin, now.Add(-2*time.Hour), "ada")
		st.addMember(room.ID, newer, roleMember, now.Add(-time.Hour), "newer")
		res, err := New(st, &tape{}).LeaveRoom(asUser(admin), room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if res.NewAdminID != older || res.Deleted {
			t.Fatalf("result %+v", res)
		}
		role, err := st.GetMemberRole(context.Background(), room.ID, older)
		if err != nil || role != roleAdmin {
			t.Fatalf("oldest role %s %v", role, err)
		}
	})

	t.Run("non-admin leave keeps the room", func(t *testing.T) {
		admin, member := uuid.New(), uuid.New()
		st := newMem()
		room := st.addRoom(admin, "Studio")
		st.addMember(room.ID, admin, roleAdmin, time.Now().Add(-time.Hour), "ada")
		st.addMember(room.ID, member, roleMember, time.Now(), "bea")
		pub := &tape{}
		res, err := New(st, pub).LeaveRoom(asUser(member), room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if res.Deleted || res.NewAdminID != uuid.Nil {
			t.Fatalf("result %+v", res)
		}
		role, err := st.GetMemberRole(context.Background(), room.ID, admin)
		if err != nil || role != roleAdmin {
			t.Fatal(role, err)
		}
		if pub.subjects[0] != subjectLeft || len(pub.subjects) != 1 {
			t.Fatalf("events %v", pub.subjects)
		}
	})

	t.Run("not a member", func(t *testing.T) {
		admin := uuid.New()
		st := newMem()
		room := st.addRoom(admin, "Studio")
		st.addMember(room.ID, admin, roleAdmin, time.Now(), "ada")
		_, err := New(st, &tape{}).LeaveRoom(asUser(uuid.New()), room.ID)
		if !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	})
}

func TestDeleteRoomAdminOnly(t *testing.T) {
	admin, member := uuid.New(), uuid.New()
	st := newMem()
	room := st.addRoom(admin, "Studio")
	st.addMember(room.ID, admin, roleAdmin, time.Now().Add(-time.Hour), "ada")
	st.addMember(room.ID, member, roleMember, time.Now(), "bea")
	pub := &tape{}
	svc := New(st, pub)

	if err := svc.DeleteRoom(asUser(member), room.ID); !errors.Is(err, ErrNotAdmin) {
		t.Fatalf("member delete: %v", err)
	}
	got, err := st.GetRoom(context.Background(), room.ID)
	if err != nil || got.Deleted {
		t.Fatalf("room changed %+v %v", got, err)
	}
	if len(pub.subjects) != 0 {
		t.Fatal("member delete published")
	}
	if err := svc.DeleteRoom(asUser(admin), room.ID); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetRoom(context.Background(), room.ID)
	if err != nil || !got.Deleted {
		t.Fatalf("not deleted %+v %v", got, err)
	}
	if pub.subjects[0] != subjectDeleted || string(pub.payloads[0]) != room.ID.String() {
		t.Fatalf("event %v %q", pub.subjects, pub.payloads[0])
	}
	if err := svc.DeleteRoom(asUser(admin), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestCheckMembershipUsesSubjectNotCaller(t *testing.T) {
	admin, member, outsider := uuid.New(), uuid.New(), uuid.New()
	st := newMem()
	room := st.addRoom(admin, "Studio")
	st.addMember(room.ID, admin, roleAdmin, time.Now().Add(-time.Hour), "ada")
	st.addMember(room.ID, member, roleMember, time.Now(), "bea")
	svc := New(st, &tape{})

	role, err := svc.CheckMembership(asUser(outsider), room.ID, member)
	if err != nil || role != roleMember {
		t.Fatalf("subject role %s %v", role, err)
	}
	role, err = svc.CheckMembership(asUser(member), room.ID, admin)
	if err != nil || role != roleAdmin {
		t.Fatalf("admin role %s %v", role, err)
	}
	role, err = svc.CheckMembership(asUser(admin), room.ID, uuid.New())
	if err != nil || role != "" {
		t.Fatalf("none %q %v", role, err)
	}
	if err := st.SoftDeleteRoom(context.Background(), room.ID); err != nil {
		t.Fatal(err)
	}
	role, err = svc.CheckMembership(asUser(admin), room.ID, member)
	if err != nil || role != "" {
		t.Fatalf("deleted %q %v", role, err)
	}
	if _, err := svc.CheckMembership(context.Background(), room.ID, member); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
}

func TestMembershipReadsAndAdminTransfer(t *testing.T) {
	admin, member := uuid.New(), uuid.New()
	st := newMem()
	st.profiles[admin] = "ada"
	room := st.addRoom(admin, "Studio")
	st.addMember(room.ID, admin, roleAdmin, time.Now().Add(-time.Hour), "ada")
	st.addMember(room.ID, member, roleMember, time.Now(), "bea")
	svc := New(st, &tape{})

	if _, err := svc.GetRoom(asUser(uuid.New()), room.ID); !errors.Is(err, ErrNotMember) {
		t.Fatal(err)
	}
	got, err := svc.GetRoom(asUser(member), room.ID)
	if err != nil || got.Title != "Studio" || got.AdminID != admin {
		t.Fatal(err, got)
	}
	if _, err := svc.ListMembers(asUser(uuid.New()), room.ID); !errors.Is(err, ErrNotMember) {
		t.Fatal(err)
	}
	members, err := svc.ListMembers(asUser(member), room.ID)
	if err != nil || len(members) != 2 || members[0].UserID != admin {
		t.Fatalf("%v %v", members, err)
	}
	if err := svc.TransferAdmin(asUser(member), room.ID, admin); !errors.Is(err, ErrNotAdmin) {
		t.Fatal(err)
	}
	if err := svc.TransferAdmin(asUser(admin), room.ID, uuid.New()); !errors.Is(err, ErrNotMember) {
		t.Fatal(err)
	}
	if err := svc.TransferAdmin(asUser(admin), room.ID, member); err != nil {
		t.Fatal(err)
	}
	role, err := st.GetMemberRole(context.Background(), room.ID, member)
	if err != nil || role != roleAdmin {
		t.Fatal(role, err)
	}
	stored, err := st.GetRoom(context.Background(), room.ID)
	if err != nil || stored.AdminID != member {
		t.Fatal(stored, err)
	}
}

func TestCreateAndValidateInvite(t *testing.T) {
	admin, member := uuid.New(), uuid.New()
	st := newMem()
	room := st.addRoom(admin, "Studio")
	st.addMember(room.ID, admin, roleAdmin, time.Now().Add(-time.Hour), "ada")
	st.addMember(room.ID, member, roleMember, time.Now(), "bea")
	svc := New(st, &tape{})

	if _, err := svc.CreateInvite(asUser(member), room.ID, time.Hour, 2); !errors.Is(err, ErrNotAdmin) {
		t.Fatal(err)
	}
	inv, err := svc.CreateInvite(asUser(admin), room.ID, time.Hour, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Token) < 16 || len(inv.Token) > 128 {
		t.Fatalf("token %q", inv.Token)
	}
	valid, roomID, err := svc.ValidateInvite(asUser(member), inv.Token)
	if err != nil || !valid || roomID != room.ID {
		t.Fatalf("valid %v %s %v", valid, roomID, err)
	}
	st.expireFirst()
	valid, _, err = svc.ValidateInvite(asUser(member), inv.Token)
	if err != nil || valid {
		t.Fatalf("expired valid=%v %v", valid, err)
	}
	if _, _, err := svc.ValidateInvite(context.Background(), inv.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
}

func (m *mem) expireFirst() {
	if len(m.invites) > 0 {
		m.invites[0].ExpiresAt = time.Now().Add(-time.Minute)
	}
}

func TestEveryRPCChecksCaller(t *testing.T) {
	svc := New(newMem(), &tape{})
	ctx := context.Background()
	id := uuid.New()
	bad := metadata.NewIncomingContext(ctx, metadata.Pairs(mdUserID, "not-a-uuid"))

	if _, err := svc.CreateRoom(ctx, "t"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err := svc.CreateRoom(bad, "t"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal(err)
	}
	if _, err := svc.GetRoom(ctx, id); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, _, err := svc.JoinRoom(ctx, id, ""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err := svc.LeaveRoom(ctx, id); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if err := svc.DeleteRoom(ctx, id); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err := svc.ListMembers(ctx, id); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if err := svc.TransferAdmin(ctx, id, id); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err := svc.CreateInvite(ctx, id, time.Hour, 1); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, _, err := svc.ValidateInvite(ctx, "0123456789abcdef"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err := svc.CheckMembership(ctx, id, id); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal(err)
	}
}
