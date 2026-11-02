// Package authn verifies Supabase access tokens.
//
// RS256 and ES256 are verified against a JWKS document cached for about 15
// minutes. HS256 is legacy mode: set Config.AllowHS256 and HMACSecret to the
// Supabase JWT secret. Leave AllowHS256 false (the default) so an HMAC token
// cannot be confused with an asymmetric key.
package authn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const defaultRefreshInterval = 15 * time.Minute

// Sentinel verification errors. Callers should use errors.Is.
var (
	ErrExpired = errors.New("token expired")
	ErrInvalid = errors.New("token invalid")
	ErrNoToken = errors.New("no token")
)

// Principal is the authenticated user taken from a verified access token.
type Principal struct {
	UserID string
	Email  string
}

// Config selects how tokens are verified.
type Config struct {
	// JWKSURL is the Supabase JWKS endpoint. Required unless AllowHS256 is set.
	JWKSURL string
	// Issuer is the expected iss claim, typically https://<ref>.supabase.co/auth/v1.
	Issuer string
	// Audience is the expected aud claim. Empty means "authenticated".
	Audience string
	// RefreshInterval is how often the JWKS cache refreshes. Zero means 15 minutes.
	RefreshInterval time.Duration
	// AllowHS256 enables legacy HMAC verification with HMACSecret.
	// This is off by default. Do not enable it for projects that sign access
	// tokens with the asymmetric Supabase signing key. The HMAC secret must be
	// the Supabase JWT secret, never a public key.
	AllowHS256 bool
	// HMACSecret is SUPABASE_JWT_SECRET, used only when AllowHS256 is true.
	HMACSecret string
}

// Verifier checks access tokens.
type Verifier interface {
	Verify(ctx context.Context, rawToken string) (Principal, error)
	// Refresh fetches the JWKS document immediately. Used after key rotation.
	Refresh(ctx context.Context) error
}

type verifier struct {
	issuer     string
	audience   string
	allowHS256 bool
	hmacSecret []byte
	cache      *jwk.Cache
	jwksURL    string
}

// New fetches the JWKS document when configured and starts its refresh loop.
// Cancel ctx to stop the background refresh; keep it alive for the verifier's lifetime.
func New(ctx context.Context, cfg Config) (Verifier, error) {
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, errors.New("authn: issuer required")
	}
	if cfg.Audience == "" {
		cfg.Audience = "authenticated"
	}
	if !cfg.AllowHS256 && strings.TrimSpace(cfg.JWKSURL) == "" {
		return nil, errors.New("authn: jwks url required")
	}
	if cfg.AllowHS256 && strings.TrimSpace(cfg.HMACSecret) == "" {
		return nil, errors.New("authn: hmac secret required when hs256 is enabled")
	}

	v := &verifier{
		issuer:     cfg.Issuer,
		audience:   cfg.Audience,
		allowHS256: cfg.AllowHS256,
		hmacSecret: []byte(cfg.HMACSecret),
		jwksURL:    cfg.JWKSURL,
	}
	if strings.TrimSpace(cfg.JWKSURL) == "" {
		return v, nil
	}

	interval := cfg.RefreshInterval
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	cache := jwk.NewCache(ctx)
	if err := cache.Register(cfg.JWKSURL, jwk.WithRefreshInterval(interval)); err != nil {
		return nil, fmt.Errorf("authn: register jwks: %w", err)
	}
	if _, err := cache.Refresh(ctx, cfg.JWKSURL); err != nil {
		return nil, fmt.Errorf("authn: fetch jwks: %w", err)
	}
	v.cache = cache
	return v, nil
}

func (v *verifier) Refresh(ctx context.Context) error {
	if v.cache == nil {
		return nil
	}
	if _, err := v.cache.Refresh(ctx, v.jwksURL); err != nil {
		return fmt.Errorf("authn: refresh jwks: %w", err)
	}
	return nil
}

func (v *verifier) Verify(ctx context.Context, rawToken string) (Principal, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return Principal{}, ErrNoToken
	}
	if err := ctx.Err(); err != nil {
		return Principal{}, err
	}

	alg, err := tokenAlgorithm(rawToken)
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}

	switch alg {
	case jwa.HS256:
		if !v.allowHS256 {
			return Principal{}, fmt.Errorf("%w: hs256 rejected", ErrInvalid)
		}
		return v.verify(rawToken, jwt.WithKey(jwa.HS256, v.hmacSecret))
	case jwa.RS256, jwa.ES256:
		return v.verifyAsymmetric(ctx, rawToken)
	default:
		return Principal{}, fmt.Errorf("%w: algorithm %s not allowed", ErrInvalid, alg)
	}
}

func (v *verifier) verifyAsymmetric(ctx context.Context, rawToken string) (Principal, error) {
	if v.cache == nil {
		return Principal{}, fmt.Errorf("%w: jwks not configured", ErrInvalid)
	}
	set, err := v.cache.Get(ctx, v.jwksURL)
	if err != nil {
		if ctx.Err() != nil {
			return Principal{}, ctx.Err()
		}
		return Principal{}, fmt.Errorf("%w: jwks: %s", ErrInvalid, err.Error())
	}
	return v.verify(rawToken, jwt.WithKeySet(set))
}

func (v *verifier) verify(rawToken string, keyOpt jwt.ParseOption) (Principal, error) {
	tok, err := jwt.Parse([]byte(rawToken),
		keyOpt,
		jwt.WithValidate(true),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired()) {
			return Principal{}, fmt.Errorf("%w: %s", ErrExpired, err.Error())
		}
		return Principal{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	if tok.Expiration().IsZero() {
		return Principal{}, fmt.Errorf("%w: missing exp", ErrInvalid)
	}
	sub := tok.Subject()
	if sub == "" {
		return Principal{}, fmt.Errorf("%w: missing sub", ErrInvalid)
	}
	return Principal{UserID: sub, Email: stringClaim(tok, "email")}, nil
}

func tokenAlgorithm(raw string) (jwa.SignatureAlgorithm, error) {
	msg, err := jws.Parse([]byte(raw))
	if err != nil {
		return "", err
	}
	sigs := msg.Signatures()
	if len(sigs) != 1 {
		return "", fmt.Errorf("expected one signature")
	}
	alg := sigs[0].ProtectedHeaders().Algorithm()
	if alg == "" {
		return "", fmt.Errorf("missing alg")
	}
	return alg, nil
}

func stringClaim(tok jwt.Token, name string) string {
	v, ok := tok.Get(name)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
