package authz

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/drakeRAGE/realtime-whiteboard/gen/go/whiteboard/realtime/v1"
)

// MaxFrame is the largest client frame the hub will decode.
const MaxFrame = 64 * 1024

// Decode unmarshals one ClientEnvelope. Oversized frames are rejected.
func Decode(b []byte) (*realtimev1.ClientEnvelope, error) {
	if len(b) > MaxFrame {
		return nil, errors.New("frame too large")
	}
	var env realtimev1.ClientEnvelope
	if err := proto.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &env, nil
}

func eventName(env *realtimev1.ClientEnvelope) string {
	if env == nil {
		return ""
	}
	ref := env.ProtoReflect()
	fd := ref.WhichOneof(ref.Descriptor().Oneofs().ByName("payload"))
	if fd == nil {
		return ""
	}
	return string(fd.Name())
}

func roomOf(env *realtimev1.ClientEnvelope) string {
	ref := env.ProtoReflect()
	fd := ref.WhichOneof(ref.Descriptor().Oneofs().ByName("payload"))
	if fd == nil {
		return ""
	}
	msg := ref.Get(fd).Message()
	field := msg.Descriptor().Fields().ByName("room_id")
	if field == nil {
		return ""
	}
	return msg.Get(field).String()
}
