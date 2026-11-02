package root

// Compile-and-runtime check that generated protobuf contracts build and
// round-trip (Phase 1 validation; see docs/CONTRACTS.md).

import (
	"testing"

	"google.golang.org/protobuf/proto"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
	roomv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/room/v1"
)

func TestGeneratedContractsRoundTrip(t *testing.T) {
	env := &realtimev1.ClientEnvelope{
		ClientMsgId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Payload: &realtimev1.ClientEnvelope_StrokeInput{
			StrokeInput: &realtimev1.StrokeInput{
				StrokeId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				RoomId:   "7f2d1a9e-4a1b-4f7e-9d7a-2c3b4e5f6a70",
				Points:   []*boardv1.Point{{X: 10, Y: 20}, {X: 11, Y: 21}},
				Color:    "#e6194b",
				Width:    4,
				Tool:     boardv1.Tool_TOOL_PEN,
			},
		},
	}

	raw, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got realtimev1.ClientEnvelope
	if err := proto.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stroke := got.GetStrokeInput()
	if stroke == nil || stroke.GetStrokeId() != env.GetStrokeInput().GetStrokeId() {
		t.Fatalf("stroke round-trip mismatch: %v", got.GetPayload())
	}
	if len(stroke.GetPoints()) != 2 {
		t.Fatalf("points round-trip mismatch: %v", stroke.GetPoints())
	}
	if role := roomv1.Role_ROLE_ADMIN; role.String() != "ROLE_ADMIN" {
		t.Fatalf("unexpected role name: %s", role.String())
	}
}
