package config

import "testing"

func TestValidateOrigin(t *testing.T) {
	ok := Config{
		HTTPAddr:      ":8080",
		RoomGRPCAddr:  "localhost:7001",
		BoardGRPCAddr: "localhost:7002",
		ChatGRPCAddr:  "localhost:7003",
		FrontendURL:   "https://app.example",
		Issuer:        "https://example.supabase.co/auth/v1",
		JWKSURL:       "https://example.supabase.co/auth/v1/.well-known/jwks.json",
	}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := ok
	bad.FrontendURL = "*"
	if err := bad.Validate(); err == nil {
		t.Fatal("expected wildcard rejection")
	}
	bad = ok
	bad.JWKSURL = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("expected jwks requirement")
	}
	bad = ok
	bad.AllowHS256 = true
	bad.JWKSURL = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("expected hmac secret requirement")
	}
}
