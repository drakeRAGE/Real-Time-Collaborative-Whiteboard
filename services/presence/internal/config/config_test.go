package config

import "testing"

func TestValidate(t *testing.T) {
	if err := (Config{HTTPAddr: ":8004"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{}).Validate(); err == nil {
		t.Fatal("expected missing http addr")
	}
}
