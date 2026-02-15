package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

const defaultJWTIssuer = "orion-x-manager"

type JWTManagerConfig struct {
	Secret     string
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Now        func() time.Time
}

type JWTManager struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

func NewJWTManager(cfg JWTManagerConfig) (*JWTManager, error) {
	secret := strings.TrimSpace(cfg.Secret)
	if secret == "" {
		return nil, fmt.Errorf("%w: jwt secret is required", ErrInvalidArgument)
	}
	if cfg.AccessTTL <= 0 {
		return nil, fmt.Errorf("%w: access token ttl must be > 0", ErrInvalidArgument)
	}
	if cfg.RefreshTTL <= 0 {
		return nil, fmt.Errorf("%w: refresh token ttl must be > 0", ErrInvalidArgument)
	}

	issuer := strings.TrimSpace(cfg.Issuer)
	if issuer == "" {
		issuer = defaultJWTIssuer
	}

	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	return &JWTManager{
		secret:     []byte(secret),
		issuer:     issuer,
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
		now:        nowFn,
	}, nil
}

func (m *JWTManager) IssueTokenPair(user User) (TokenPair, error) {
	now := m.now().UTC()

	accessToken, err := m.issueToken(user, TokenTypeAccess, now, m.accessTTL)
	if err != nil {
		return TokenPair{}, err
	}
	refreshToken, err := m.issueToken(user, TokenTypeRefresh, now, m.refreshTTL)
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		AccessExpiresIn:  int64(m.accessTTL.Seconds()),
		RefreshExpiresIn: int64(m.refreshTTL.Seconds()),
	}, nil
}

func (m *JWTManager) Parse(token string, expectedType TokenType) (TokenClaims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return TokenClaims{}, ErrUnauthorized
	}

	claims := &jwtClaims{}
	parsedToken, err := jwt.ParseWithClaims(
		token,
		claims,
		func(parsed *jwt.Token) (any, error) {
			if parsed.Method == nil || parsed.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, ErrUnauthorized
			}
			return m.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
	)
	if err != nil {
		if isJWTValidationError(err) {
			return TokenClaims{}, ErrUnauthorized
		}
		return TokenClaims{}, fmt.Errorf("parse jwt: %w", err)
	}
	if !parsedToken.Valid {
		return TokenClaims{}, ErrUnauthorized
	}

	if claims.TokenType != string(expectedType) {
		return TokenClaims{}, ErrUnauthorized
	}
	if claims.Subject == "" {
		return TokenClaims{}, ErrUnauthorized
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return TokenClaims{}, ErrUnauthorized
	}

	role := contracts.UserRole(claims.Role)
	if !isSupportedRole(role) {
		return TokenClaims{}, ErrUnauthorized
	}

	var issuedAt time.Time
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.Time
	}
	var expiresAt time.Time
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}

	return TokenClaims{
		UserID:    userID,
		Role:      role,
		TokenType: expectedType,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}, nil
}

func (m *JWTManager) issueToken(user User, tokenType TokenType, now time.Time, ttl time.Duration) (string, error) {
	if !isSupportedRole(user.Role) {
		return "", fmt.Errorf("%w: unsupported user role %q", ErrInvalidArgument, user.Role)
	}
	if user.ID == uuid.Nil {
		return "", fmt.Errorf("%w: user id is required", ErrInvalidArgument)
	}

	claims := jwtClaims{
		Role:      string(user.Role),
		TokenType: string(tokenType),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt token: %w", err)
	}
	return signed, nil
}

type jwtClaims struct {
	Role      string `json:"role"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

func isJWTValidationError(err error) bool {
	return errors.Is(err, jwt.ErrTokenMalformed) ||
		errors.Is(err, jwt.ErrTokenExpired) ||
		errors.Is(err, jwt.ErrTokenNotValidYet) ||
		errors.Is(err, jwt.ErrTokenSignatureInvalid) ||
		errors.Is(err, jwt.ErrTokenUnverifiable) ||
		errors.Is(err, jwt.ErrTokenInvalidClaims)
}
