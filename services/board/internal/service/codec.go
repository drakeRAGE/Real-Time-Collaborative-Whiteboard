package service

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	boardv1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/board/v1"
	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/repo"
)

type point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type strokeBody struct {
	Points []point `json:"points"`
	Color  string  `json:"color"`
	Width  float64 `json:"width"`
	Tool   string  `json:"tool"`
}

type shapeBody struct {
	Shape string  `json:"shape"`
	Start point   `json:"start"`
	End   point   `json:"end"`
	Color string  `json:"color"`
	Width float64 `json:"width"`
}

// BoardProto converts one stored op into the history/snapshot message.
func BoardProto(op repo.Op) (*boardv1.BoardOp, error) {
	created := timestamppb.New(op.CreatedAt)
	switch op.Kind {
	case repo.KindStroke:
		var body strokeBody
		if err := json.Unmarshal(op.Payload, &body); err != nil {
			return nil, fmt.Errorf("stroke payload: %w", err)
		}
		return &boardv1.BoardOp{
			CreatedAt: created,
			Op: &boardv1.BoardOp_StrokeAdded{StrokeAdded: &boardv1.StrokeAdded{
				Seq: op.Seq,
				Stroke: &boardv1.Stroke{
					Id:     op.ID,
					RoomId: op.RoomID.String(),
					UserId: op.UserID.String(),
					Points: protoPoints(body.Points),
					Color:  body.Color,
					Width:  body.Width,
					Tool:   protoTool(body.Tool),
				},
			}},
		}, nil
	case repo.KindShape:
		var body shapeBody
		if err := json.Unmarshal(op.Payload, &body); err != nil {
			return nil, fmt.Errorf("shape payload: %w", err)
		}
		return &boardv1.BoardOp{
			CreatedAt: created,
			Op: &boardv1.BoardOp_ShapeAdded{ShapeAdded: &boardv1.ShapeAdded{
				Seq: op.Seq,
				Shape: &boardv1.Shape{
					Id:     op.ID,
					RoomId: op.RoomID.String(),
					UserId: op.UserID.String(),
					Kind:   protoShape(body.Shape),
					Start:  &boardv1.Point{X: body.Start.X, Y: body.Start.Y},
					End:    &boardv1.Point{X: body.End.X, Y: body.End.Y},
					Color:  body.Color,
					Width:  body.Width,
				},
			}},
		}, nil
	case repo.KindClear:
		return &boardv1.BoardOp{
			CreatedAt: created,
			Op: &boardv1.BoardOp_BoardCleared{BoardCleared: &boardv1.BoardCleared{
				OpId:   op.ID,
				RoomId: op.RoomID.String(),
				UserId: op.UserID.String(),
				Seq:    op.Seq,
			}},
		}, nil
	case repo.KindUndo:
		return &boardv1.BoardOp{
			CreatedAt: created,
			Op: &boardv1.BoardOp_UndoApplied{UndoApplied: &boardv1.UndoApplied{
				OpId:   payloadID(op.Payload),
				RoomId: op.RoomID.String(),
				UserId: op.UserID.String(),
				Seq:    op.Seq,
			}},
		}, nil
	case repo.KindRedo:
		return &boardv1.BoardOp{
			CreatedAt: created,
			Op: &boardv1.BoardOp_RedoApplied{RedoApplied: &boardv1.RedoApplied{
				OpId:   payloadID(op.Payload),
				RoomId: op.RoomID.String(),
				UserId: op.UserID.String(),
				Seq:    op.Seq,
			}},
		}, nil
	default:
		return nil, fmt.Errorf("unknown kind %s", op.Kind)
	}
}

// Encode marshals op as a protobuf BoardOp for the Redis hot list.
func Encode(op repo.Op) ([]byte, error) {
	msg, err := BoardProto(op)
	if err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encode op: %w", err)
	}
	return raw, nil
}

func protoPoints(in []point) []*boardv1.Point {
	out := make([]*boardv1.Point, 0, len(in))
	for _, p := range in {
		out = append(out, &boardv1.Point{X: p.X, Y: p.Y})
	}
	return out
}

func protoTool(name string) boardv1.Tool {
	if name == "eraser" {
		return boardv1.Tool_TOOL_ERASER
	}
	return boardv1.Tool_TOOL_PEN
}

func protoShape(name string) boardv1.ShapeKind {
	switch name {
	case "rect":
		return boardv1.ShapeKind_SHAPE_KIND_RECT
	case "circle":
		return boardv1.ShapeKind_SHAPE_KIND_CIRCLE
	default:
		return boardv1.ShapeKind_SHAPE_KIND_LINE
	}
}

func toolName(tool boardv1.Tool) string {
	if tool == boardv1.Tool_TOOL_ERASER {
		return "eraser"
	}
	return "pen"
}

func shapeName(kind boardv1.ShapeKind) string {
	switch kind {
	case boardv1.ShapeKind_SHAPE_KIND_RECT:
		return "rect"
	case boardv1.ShapeKind_SHAPE_KIND_CIRCLE:
		return "circle"
	default:
		return "line"
	}
}

func pointsFrom(in []*boardv1.Point) []point {
	out := make([]point, 0, len(in))
	for _, p := range in {
		if p == nil {
			continue
		}
		out = append(out, point{X: p.GetX(), Y: p.GetY()})
	}
	return out
}
