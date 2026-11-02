package authz

import (
	"testing"

	"buf.build/go/protovalidate"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func FuzzDecodeEnvelope(f *testing.F) {
	v, err := protovalidate.New()
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte(nil))
	f.Add([]byte{0xff, 0x00, 0x01})
	f.Add([]byte("not-a-protobuf"))
	f.Fuzz(func(t *testing.T, data []byte) {
		env, err := Decode(data)
		if err != nil || env == nil {
			return
		}
		_ = eventName(env)
		_ = roomOf(env)
		_ = v.Validate(env)
	})
}
