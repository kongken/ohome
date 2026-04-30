package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/kongken/ohome/internal/config"
)

// TokenKind tags an issued token so refresh tokens can't be used in place
// of access tokens and vice versa.
type TokenKind string

const (
	AccessToken  TokenKind = "access"
	RefreshToken TokenKind = "refresh"
)

const (
	defaultAccessTTL  = time.Hour
	defaultRefreshTTL = 30 * 24 * time.Hour
)

// Claims is what we sign into both access and refresh JWTs.
type Claims struct {
	Username string    `json:"username,omitempty"`
	Kind     TokenKind `json:"kind"`
	jwt.RegisteredClaims
}

// Issuer signs and verifies JWTs. Build one per process from
// ServiceConfig.Auth via NewIssuer.
type Issuer struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewIssuer returns a configured JWT issuer. Falls back to sensible TTLs
// when the config strings are empty/invalid.
func NewIssuer(cfg config.AuthConfig) (*Issuer, error) {
	if cfg.JWTSecret == "" {
		return nil, errors.New("auth: jwt_secret is required")
	}
	access := parseDurationOr(cfg.AccessTokenTTL, defaultAccessTTL)
	refresh := parseDurationOr(cfg.RefreshTokenTTL, defaultRefreshTTL)
	return &Issuer{
		secret:     []byte(cfg.JWTSecret),
		accessTTL:  access,
		refreshTTL: refresh,
	}, nil
}

// IssuePair signs an access+refresh token for the given user.
func (i *Issuer) IssuePair(userID, username string) (access, refresh string, accessExpiresIn int64, err error) {
	now := time.Now()
	access, err = i.sign(userID, username, AccessToken, now, now.Add(i.accessTTL))
	if err != nil {
		return "", "", 0, err
	}
	refresh, err = i.sign(userID, username, RefreshToken, now, now.Add(i.refreshTTL))
	if err != nil {
		return "", "", 0, err
	}
	return access, refresh, int64(i.accessTTL.Seconds()), nil
}

// IssueAccess signs only an access token (used after refresh).
func (i *Issuer) IssueAccess(userID, username string) (string, int64, error) {
	now := time.Now()
	tok, err := i.sign(userID, username, AccessToken, now, now.Add(i.accessTTL))
	if err != nil {
		return "", 0, err
	}
	return tok, int64(i.accessTTL.Seconds()), nil
}

// Parse verifies the token and returns the claims. Returns an error with
// `ErrTokenExpired` semantics when the token is past expiry — caller maps
// to httpx.CodeAuthTokenExpired.
func (i *Issuer) Parse(token string, want TokenKind) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return i.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Kind != want {
		return nil, fmt.Errorf("wrong token kind: got %q want %q", claims.Kind, want)
	}
	return claims, nil
}

func (i *Issuer) sign(userID, username string, kind TokenKind, issuedAt, expiresAt time.Time) (string, error) {
	c := &Claims{
		Username: username,
		Kind:     kind,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Issuer:    "ohome",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString(i.secret)
}

func parseDurationOr(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
