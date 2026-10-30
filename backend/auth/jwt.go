package auth

import "github.com/golang-jwt/jwt/v5"

type Claims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Name   string `json:"name"`
	MFA    bool   `json:"mfa"`
	jwt.RegisteredClaims
}
