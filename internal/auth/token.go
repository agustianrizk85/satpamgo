package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
	Type   string `json:"type"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret []byte
	issuer string
}

func NewTokenService(secret, issuer string) *TokenService {
	return &TokenService{
		secret: []byte(secret),
		issuer: issuer,
	}
}

func (s *TokenService) signToken(userID, role, tokenType string, ttl time.Duration) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		Role:   role,
		Type:   tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	})

	return token.SignedString(s.secret)
}

func (s *TokenService) Sign(userID, role string) (string, error) {
	return s.signToken(userID, role, "access", 8*time.Hour)
}

func (s *TokenService) SignWithTTL(userID, role string, ttl time.Duration) (string, error) {
	return s.signToken(userID, role, "access", ttl)
}

func (s *TokenService) SignRefresh(userID, role string) (string, error) {
	return s.signToken(userID, role, "refresh", 30*24*time.Hour)
}

func (s *TokenService) SignRefreshWithTTL(userID, role string, ttl time.Duration) (string, error) {
	return s.signToken(userID, role, "refresh", ttl)
}

func (s *TokenService) Verify(raw string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.UserID == "" || claims.Role == "" || claims.Type == "" {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}

func (s *TokenService) VerifyAccess(raw string) (*Claims, error) {
	claims, err := s.Verify(raw)
	if err != nil {
		return nil, err
	}
	if claims.Type != "access" {
		return nil, errors.New("invalid access token")
	}
	return claims, nil
}

func (s *TokenService) VerifyRefresh(raw string) (*Claims, error) {
	claims, err := s.Verify(raw)
	if err != nil {
		return nil, err
	}
	if claims.Type != "refresh" {
		return nil, errors.New("invalid refresh token")
	}
	return claims, nil
}
