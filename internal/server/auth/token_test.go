package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewTokenServiceRequiresSecret(t *testing.T) {
	_, err := NewTokenService("   ", 0)
	if !errors.Is(err, ErrSecretRequired) {
		t.Fatalf("expected ErrSecretRequired, got %v", err)
	}
}

func TestTokenServiceSignAndVerifyRoundTrip(t *testing.T) {
	now := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)
	service, err := newTokenService("test-secret", time.Second, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	token, err := service.Sign(Claims{
		Subject:     "player-1",
		MatchID:     "match-000001",
		SessionKind: "player",
		Scope:       ScopeGameplay,
		IssuedAt:    now.Unix(),
		ExpiresAt:   now.Add(2 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign claims: %v", err)
	}
	if !strings.HasPrefix(token, tokenVersion+".") {
		t.Fatalf("expected token to include version prefix, got %q", token)
	}

	verified, err := service.Verify(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if verified.Subject != "player-1" {
		t.Fatalf("expected subject player-1, got %q", verified.Subject)
	}
	if verified.MatchID != "match-000001" {
		t.Fatalf("expected match id match-000001, got %q", verified.MatchID)
	}
	if verified.SessionKind != "player" {
		t.Fatalf("expected session kind player, got %q", verified.SessionKind)
	}
	if !verified.Allows(ScopeGameplay) {
		t.Fatalf("expected gameplay scope allowance")
	}
}

func TestClaimsAllowsAdminForAllScopes(t *testing.T) {
	admin := Claims{Scope: ScopeAdmin}
	if !admin.Allows(ScopeAdmin) || !admin.Allows(ScopeLobby) || !admin.Allows(ScopeGameplay) {
		t.Fatalf("expected admin scope to allow all scopes")
	}
}

func TestClaimsAllowsGameplayScopeToReadLobbies(t *testing.T) {
	gameplay := Claims{Scope: ScopeGameplay}
	if !gameplay.Allows(ScopeLobby) {
		t.Fatalf("expected gameplay scope to include lobby_read capability")
	}
}

func TestTokenServiceVerifyRejectsTamperedToken(t *testing.T) {
	now := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)
	service, err := newTokenService("test-secret", 0, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	token, err := service.Sign(Claims{
		Subject:     "player-1",
		SessionKind: "player",
		Scope:       ScopeGameplay,
		IssuedAt:    now.Unix(),
		ExpiresAt:   now.Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign claims: %v", err)
	}

	tampered := token[:len(token)-1] + "x"
	_, err = service.Verify(tampered)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for tampered token, got %v", err)
	}
}

func TestTokenServiceVerifyRejectsExpiredToken(t *testing.T) {
	base := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)
	now := base.Add(2 * time.Minute)
	service, err := newTokenService("test-secret", 0, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	token, err := service.Sign(Claims{
		Subject:     "player-1",
		SessionKind: "player",
		Scope:       ScopeGameplay,
		IssuedAt:    base.Unix(),
		ExpiresAt:   base.Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign claims: %v", err)
	}

	_, err = service.Verify(token)
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestTokenServiceVerifyRejectsNotYetValidToken(t *testing.T) {
	now := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)
	service, err := newTokenService("test-secret", 0, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	token, err := service.Sign(Claims{
		Subject:     "player-1",
		SessionKind: "player",
		Scope:       ScopeGameplay,
		IssuedAt:    now.Add(time.Minute).Unix(),
		ExpiresAt:   now.Add(2 * time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign claims: %v", err)
	}

	_, err = service.Verify(token)
	if !errors.Is(err, ErrTokenNotYetValid) {
		t.Fatalf("expected ErrTokenNotYetValid, got %v", err)
	}
}

func TestTokenServiceSignRejectsInvalidExpiryWindow(t *testing.T) {
	now := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)
	service, err := newTokenService("test-secret", 0, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	_, err = service.Sign(Claims{
		Subject:     "player-1",
		SessionKind: "player",
		Scope:       ScopeGameplay,
		IssuedAt:    now.Unix(),
		ExpiresAt:   now.Unix(),
	})
	if !errors.Is(err, ErrInvalidClaims) {
		t.Fatalf("expected ErrInvalidClaims, got %v", err)
	}
}
