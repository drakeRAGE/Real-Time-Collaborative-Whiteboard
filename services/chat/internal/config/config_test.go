package config

import "testing"

func TestValidate(t *testing.T) {
	if err := (Config{GRPCAddr: ":7003", HTTPAddr: ":8003"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{HTTPAddr: ":8003"}).Validate(); err == nil {
		t.Fatal("expected missing grpc addr")
	}
	if err := (Config{GRPCAddr: ":7003"}).Validate(); err == nil {
		t.Fatal("expected missing http addr")
	}
}
