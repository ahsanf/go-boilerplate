package common

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"go-boilerplate/configs"
	"go-boilerplate/internal/utils"
	"go-boilerplate/internal/utils/apperror"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// GlobalAuthMiddleware validates an Authorization: Bearer token.
//
// Flow (mirrors global_auth.ts):
//  1. Require Authorization header.
//  2. Allow STATIC_TOKEN bypass.
//  3. Parse optional 4th JWT segment that carries platform/role metadata.
//  4. Verify via Firebase Admin SDK; on failure fall back to RS256 service-account JWT.
//  5. Sets c.Locals("auth_user", AuthUser{...}) and c.Locals("email", email).
func GlobalAuthMiddleware(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return authError("Authorization header is missing")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
		return authError("Invalid authorization format")
	}

	token := parts[1]
	c.Locals("original_token", token)

	// Static-token bypass
	if st := configs.Cfg.StaticToken; st != "" && st == token {
		return c.Next()
	}

	// Parse optional 4th segment (base64-JSON) carrying platform metadata
	var currentRole, userId, ModuleId, workunitId string
	segments := strings.Split(token, ".")
	if len(segments) > 3 {
		raw, err := base64.RawStdEncoding.DecodeString(segments[3])
		if err == nil {
			var meta struct {
				CurrentRole string `json:"currentRole"`
				UserId      string `json:"userId"`
				ModuleId  string `json:"ModuleId"`
				WorkunitId  string `json:"workunitId"`
			}
			if json.Unmarshal(raw, &meta) == nil {
				currentRole = meta.CurrentRole
				userId = meta.UserId
				ModuleId = meta.ModuleId
				workunitId = meta.WorkunitId
			}
		}
		token = segments[0] + "." + segments[1] + "." + segments[2]
	}

	email, err := getEmailFromToken(c.Context(), token)
	if err != nil {
		return authError(err.Error())
	}

	c.Locals("auth_user", AuthUser{
		Email:      email,
		Role:       currentRole,
		UserId:     userId,
		ModuleId: ModuleId,
		WorkunitId: workunitId,
		Token:      token,
	})

	return c.Next()
}

// HybridTokenMiddleware is an alias for GlobalAuthMiddleware kept for naming consistency.
var HybridTokenMiddleware = GlobalAuthMiddleware

// AuthTokenMiddleware validates our own HS256 JWT (SecretKey).
// Use this for internal services that issue their own tokens.
func AuthTokenMiddleware(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return authError("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return authError("invalid authorization header format")
	}

	claims, err := utils.ValidateToken(parts[1])
	if err != nil {
		return authError("invalid or expired token")
	}

	c.Locals("user_id", claims.UserID)
	c.Locals("email", claims.Email)
	return c.Next()
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func authError(msg string) error {
	return apperror.New(msg).Unauthorized()
}

// getEmailFromToken tries Firebase first, then falls back to service-account RS256 JWT.
func getEmailFromToken(ctx context.Context, token string) (string, error) {
	// Firebase Admin SDK
	if configs.FirebaseAuth != nil {
		decoded, err := configs.FirebaseAuth.VerifyIDToken(ctx, token)
		if err == nil {
			if email, ok := decoded.Claims["email"].(string); ok && email != "" {
				return email, nil
			}
		}
	}

	// Service-account RS256 fallback
	if configs.Cfg.ServiceAccount != "" {
		return verifyWithServiceAccount(token)
	}

	return "", errors.New("invalid access token")
}

type saFileClaims struct {
	Email    string `json:"email"`
	Firebase struct {
		Identities struct {
			Email []string `json:"email"`
		} `json:"identities"`
	} `json:"firebase"`
	jwt.RegisteredClaims
}

func verifyWithServiceAccount(token string) (string, error) {
	data, err := os.ReadFile(configs.Cfg.ServiceAccount)
	if err != nil {
		return "", fmt.Errorf("cannot read service account: %w", err)
	}

	var sa struct {
		PrivateKey string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &sa); err != nil {
		return "", fmt.Errorf("invalid service account JSON: %w", err)
	}

	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return "", errors.New("failed to decode PEM private key")
	}
	keyIface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	rsaKey, ok := keyIface.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("service account key is not RSA")
	}

	var claims saFileClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected alg: %v", t.Header["alg"])
		}
		return &rsaKey.PublicKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			email := emailFromClaims(&claims)
			if ok, _ := verifyRefreshToken(email); ok {
				return email, nil
			}
		}
		return "", apperror.New(err.Error()).Unauthorized()
	}
	if !parsed.Valid {
		return "", errors.New("invalid token")
	}

	return emailFromClaims(&claims), nil
}

func emailFromClaims(c *saFileClaims) string {
	if c.Email != "" {
		return c.Email
	}
	if len(c.Firebase.Identities.Email) > 0 {
		return c.Firebase.Identities.Email[0]
	}
	return ""
}

func verifyRefreshToken(email string) (bool, error) {
	url := configs.Cfg.AuthAPIURL
	if url == "" {
		return false, nil
	}
	body, _ := json.Marshal(map[string]string{"email": email, "origin": "oba"})
	resp, err := http.Post(url+"/auth/sso/validate-refresh-token", "application/json", bytes.NewReader(body))
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	var res struct {
		Success bool `json:"success"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	return res.Success, nil
}

func splitRole(role string) []string {
	if role == "" {
		return nil
	}
	return strings.Split(role, ",")
}

// authErrorResponse returns the canonical 401 JSON shape.
// Used by permission.go as well.
func authErrorResponse(msg string) fiber.Map {
	return fiber.Map{
		"success": false,
		"error": fiber.Map{
			"type":       "APP",
			"code":       "UNAUTHORIZED",
			"statusCode": 401,
			"message":    msg,
		},
	}
}
