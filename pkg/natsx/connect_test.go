package natsx

import (
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

func applyOptions(t *testing.T, opts []nats.Option) nats.Options {
	t.Helper()
	o := nats.GetDefaultOptions()
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			t.Fatal(err)
		}
	}
	return o
}

func TestConnectOptionsWithoutCreds(t *testing.T) {
	opts, err := ConnectOptions("board", "")
	if err != nil {
		t.Fatal(err)
	}
	o := applyOptions(t, opts)
	if o.Name != "board" {
		t.Fatalf("name %q", o.Name)
	}
	if o.UserJWT != nil || o.SignatureCB != nil {
		t.Fatal("expected no credential callbacks")
	}
}

func TestConnectOptionsWithCreds(t *testing.T) {
	kp, err := nkeys.CreateUser()
	if err != nil {
		t.Fatal(err)
	}
	seed, err := kp.Seed()
	if err != nil {
		t.Fatal(err)
	}
	const userJWT = "eyJhbGciOiJlZDI1NTE5LW5rZXkifQ.test.sig"
	creds := "-----BEGIN NATS USER JWT-----\n" + userJWT + "\n------END NATS USER JWT------\n\n" +
		"-----BEGIN USER NKEY SEED-----\n" + string(seed) + "\n------END USER NKEY SEED------\n"

	opts, err := ConnectOptions("gateway", creds)
	if err != nil {
		t.Fatal(err)
	}
	o := applyOptions(t, opts)
	if o.Name != "gateway" {
		t.Fatalf("name %q", o.Name)
	}
	if o.UserJWT == nil || o.SignatureCB == nil {
		t.Fatal("expected credential callbacks")
	}
	got, err := o.UserJWT()
	if err != nil || got != userJWT {
		t.Fatalf("jwt %q err %v", got, err)
	}
	nonce := []byte("server-nonce")
	sig, err := o.SignatureCB(nonce)
	if err != nil {
		t.Fatal(err)
	}
	if err := kp.Verify(nonce, sig); err != nil {
		t.Fatalf("signature does not verify with the creds seed: %v", err)
	}
}

func TestConnectOptionsInvalidCreds(t *testing.T) {
	if _, err := ConnectOptions("chat", "not a creds file"); err == nil {
		t.Fatal("expected error for creds without a seed")
	}
}
