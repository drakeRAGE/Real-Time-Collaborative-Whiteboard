package config

import "testing"

func TestValidate(t *testing.T) {
	t.Parallel()
	if err := (Config{GRPCAddr: ":7001", HTTPAddr: ":8001"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{HTTPAddr: ":8001"}).Validate(); err == nil {
		t.Fatal("expected error")
	}
}
