package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	TypSession = "session"
	TypMedia   = "media"
	TypSSE     = "sse"
)

type Claims struct {
	Sub  int64  `json:"sub"`
	User string `json:"username"`
	Role string `json:"role"`
	Typ  string `json:"typ"`
	jwt.RegisteredClaims
}

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func VerifyPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func IssueToken(secret string, u User) (string, error) {
	return encode(secret, Claims{
		Sub:  u.ID,
		User: u.Username,
		Role: u.Role,
		Typ:  TypSession,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
}

func IssueTicket(secret string, u User, typ string, ttl time.Duration) (token string, exp int64, err error) {
	if typ != TypMedia && typ != TypSSE {
		return "", 0, errors.New("无效的 ticket 类型")
	}
	expires := time.Now().Add(ttl)
	tok, err := encode(secret, Claims{
		Sub:  u.ID,
		User: u.Username,
		Role: u.Role,
		Typ:  typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	if err != nil {
		return "", 0, err
	}
	return tok, expires.Unix(), nil
}

func Parse(secret, token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Typ == "" {
		claims.Typ = TypSession
	}
	return claims, nil
}

func encode(secret string, claims Claims) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}
