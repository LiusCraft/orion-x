package main

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signToken(secret []byte, userID string, isAdmin bool) (string, error) {
	claims := jwt.MapClaims{
		"sub":      userID,
		"is_admin": isAdmin,
		"exp":      jwt.NewNumericDate(timeNow().Add(24 * time.Hour)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return tok, nil
}
