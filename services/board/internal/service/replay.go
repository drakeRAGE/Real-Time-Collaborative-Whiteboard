package service

import (
	"encoding/json"

	"github.com/drakeRAGE/realtime-whiteboard/services/board/internal/repo"
)

// replay walks the op-log. visible is the effective board. undo and redo are
// stacks with the top at the end. A stroke, shape, or clear clears redo.
func replay(ops []repo.Op) (visible, undo, redo []repo.Op) {
	byID := make(map[string]repo.Op, len(ops))
	active := make(map[string]bool, len(ops))
	var undoIDs []string
	var redoIDs []string
	for _, op := range ops {
		byID[op.ID] = op
		switch op.Kind {
		case repo.KindStroke, repo.KindShape, repo.KindClear:
			active[op.ID] = true
			undoIDs = append(undoIDs, op.ID)
			redoIDs = nil
		case repo.KindUndo:
			id := payloadID(op.Payload)
			active[id] = false
			undoIDs = removeID(undoIDs, id)
			redoIDs = append(redoIDs, id)
		case repo.KindRedo:
			id := payloadID(op.Payload)
			active[id] = true
			redoIDs = removeID(redoIDs, id)
			undoIDs = append(undoIDs, id)
		}
	}
	var clearSeq int64
	for _, id := range undoIDs {
		op := byID[id]
		if op.Kind == repo.KindClear && op.Seq > clearSeq {
			clearSeq = op.Seq
		}
	}
	for _, op := range ops {
		if (op.Kind == repo.KindStroke || op.Kind == repo.KindShape) && active[op.ID] && op.Seq > clearSeq {
			visible = append(visible, op)
		}
	}
	undo = idsToOps(byID, undoIDs)
	redo = idsToOps(byID, redoIDs)
	return visible, undo, redo
}

func effective(ops []repo.Op) []repo.Op {
	visible, _, _ := replay(ops)
	return visible
}

func payloadID(raw json.RawMessage) string {
	var body struct {
		OpID string `json:"op_id"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	return body.OpID
}

func removeID(ids []string, id string) []string {
	for i := len(ids) - 1; i >= 0; i-- {
		if ids[i] == id {
			out := make([]string, 0, len(ids)-1)
			out = append(out, ids[:i]...)
			out = append(out, ids[i+1:]...)
			return out
		}
	}
	return ids
}

func idsToOps(byID map[string]repo.Op, ids []string) []repo.Op {
	out := make([]repo.Op, 0, len(ids))
	for _, id := range ids {
		op, ok := byID[id]
		if ok {
			out = append(out, op)
		}
	}
	return out
}
