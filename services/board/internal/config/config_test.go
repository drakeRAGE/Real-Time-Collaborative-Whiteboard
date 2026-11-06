package config

import "testing"

func TestValidate(t *testing.T) {
	ok := Config{GRPCAddr: ":7002", HTTPAddr: ":8002", RoomGRPCAddr: "room:7001"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{HTTPAddr: ":8002", RoomGRPCAddr: "room:7001"}).Validate(); err == nil {
		t.Fatal("expected missing grpc")
	}
	if err := (Config{GRPCAddr: ":7002", RoomGRPCAddr: "room:7001"}).Validate(); err == nil {
		t.Fatal("expected missing http")
	}
	if err := (Config{GRPCAddr: ":7002", HTTPAddr: ":8002"}).Validate(); err == nil {
		t.Fatal("expected missing room addr")
	}
}
