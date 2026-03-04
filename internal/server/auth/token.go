package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const tokenVersion = "v1"

type Scope string

const (
	ScopeGameplay Scope = "gameplay"
	ScopeLobby    Scope = "lobby_read"
	ScopeAdmin    Scope = "admin"
)

var (
	ErrSecretRequired    = errors.New("auth: secret is required")
	ErrInvalidToken      = errors.New("auth: invalid token")
	ErrInvalidSignature  = errors.New("auth: invalid signature")
	ErrInvalidClaims     = errors.New("auth: invalid claims")
	ErrTokenExpired      = errors.New("auth: token expired")
	ErrTokenNotYetValid  = errors.New("auth: token not yet valid")
	ErrUnsupportedFormat = errors.New("auth: unsupported token format")
)

type Claims struct {
	Subject     string `json:"sub,omitempty"`
	MatchID     string `json:"mid,omitempty"`
	SessionKind string `json:"kind,omitempty"`
	Scope       Scope  `json:"scope,omitempty"`
	IssuedAt    int64  `json:"iat"`
	ExpiresAt   int64  `json:"exp"`
}

func (c Claims) Allows(scope Scope) bool {
	if c.Scope == ScopeAdmin {
		return true
	}
	if scope == ScopeLobby && c.Scope == ScopeGameplay {
		return true
	}
	return c.Scope == scope
}

type TokenService struct {
	secret    []byte
	clockSkew time.Duration
	now       func() time.Time
}

func NewTokenService(secret string, clockSkew time.Duration) (*TokenService, error) {
	return newTokenService(secret, clockSkew, time.Now)
}

func newTokenService(secret string, clockSkew time.Duration, now func() time.Time) (*TokenService, error) {
	trimmedSecret := strings.TrimSpace(secret)
	if trimmedSecret == "" {
		return nil, ErrSecretRequired
	}
	if clockSkew < 0 {
		clockSkew = 0
	}
	if now == nil {
		now = time.Now
	}

	return &TokenService{
		secret:    []byte(trimmedSecret),
		clockSkew: clockSkew,
		now:       now,
	}, nil
}

func (s *TokenService) Sign(claims Claims) (string, error) {
	if s == nil || len(s.secret) == 0 {
		return "", ErrSecretRequired
	}

	normalized := claims
	if normalized.IssuedAt <= 0 {
		normalized.IssuedAt = s.now().UTC().Unix()
	}
	if normalized.ExpiresAt <= 0 || normalized.ExpiresAt <= normalized.IssuedAt {
		return "", ErrInvalidClaims
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("auth: marshal claims: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := tokenVersion + "." + payloadB64
	signature := s.computeSignature(signingInput)
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + signatureB64, nil
}

func (s *TokenService) Verify(token string) (Claims, error) {
	if s == nil || len(s.secret) == 0 {
		return Claims{}, ErrSecretRequired
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	if parts[0] != tokenVersion {
		return Claims{}, ErrUnsupportedFormat
	}

	signingInput := parts[0] + "." + parts[1]
	givenSignature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	expectedSignature := s.computeSignature(signingInput)
	if !hmac.Equal(givenSignature, expectedSignature) {
		return Claims{}, ErrInvalidSignature
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.ExpiresAt <= 0 || claims.ExpiresAt <= claims.IssuedAt {
		return Claims{}, ErrInvalidClaims
	}

	nowUnix := s.now().UTC().Unix()
	skewSeconds := int64(s.clockSkew / time.Second)
	if claims.IssuedAt > 0 && nowUnix+skewSeconds < claims.IssuedAt {
		return Claims{}, ErrTokenNotYetValid
	}
	if nowUnix-skewSeconds >= claims.ExpiresAt {
		return Claims{}, ErrTokenExpired
	}

	return claims, nil
}

func (s *TokenService) computeSignature(signingInput string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}
