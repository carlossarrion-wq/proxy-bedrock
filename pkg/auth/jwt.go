package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWTConfig contiene la configuración para JWT
type JWTConfig struct {
	SecretKey string
	Issuer    string
	Audience  string
}

// JWTClaims representa los claims personalizados del JWT
type JWTClaims struct {
	jwt.RegisteredClaims
	UserID                  string   `json:"user_id"`
	Email                   string   `json:"email"`
	IAMUsername             string   `json:"iam_username"`
	IAMGroups               []string `json:"iam_groups"`
	DefaultInferenceProfile string   `json:"default_inference_profile"`
	Team                    string   `json:"team,omitempty"`
	Person                  string   `json:"person,omitempty"`
}

// CreateToken genera un nuevo JWT (útil para testing)
func CreateToken(config JWTConfig, userID, email, iamUsername string, iamGroups []string) (string, string, error) {
	jti := uuid.New().String()
	now := time.Now()
	exp := now.AddDate(1, 0, 0) // 1 año

	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			Issuer:    config.Issuer,
			Audience:  jwt.ClaimStrings{config.Audience},
		},
		UserID:                  userID,
		Email:                   email,
		IAMUsername:             iamUsername,
		IAMGroups:               iamGroups,
		DefaultInferenceProfile: "us.anthropic.claude-sonnet-4-5-v2:0",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(config.SecretKey))
	if err != nil {
		return "", "", fmt.Errorf("error signing token: %w", err)
	}

	return tokenString, jti, nil
}

// ValidateToken valida un JWT y retorna los claims
func ValidateToken(tokenString, secretKey string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verificar que el algoritmo sea HMAC
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("error parsing token: %w", err)
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		// Verificar expiración adicional (por si acaso)
		if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
			return nil, fmt.Errorf("token expired")
		}
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// HashToken genera SHA256 hash del token para almacenar/buscar en BD
func HashToken(tokenString string) string {
	hash := sha256.Sum256([]byte(tokenString))
	return hex.EncodeToString(hash[:])
}

// ExtractBearerToken extrae el token del header Authorization
func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", fmt.Errorf("authorization header is empty")
	}

	// Formato esperado: "Bearer <token>"
	const bearerPrefix = "Bearer "
	if len(authHeader) < len(bearerPrefix) {
		return "", fmt.Errorf("invalid authorization header format")
	}

	if authHeader[:len(bearerPrefix)] != bearerPrefix {
		return "", fmt.Errorf("authorization header must start with 'Bearer '")
	}

	token := authHeader[len(bearerPrefix):]
	if token == "" {
		return "", fmt.Errorf("token is empty")
	}

	return token, nil
}