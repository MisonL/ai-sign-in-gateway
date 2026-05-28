package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"ai-sign-in-gateway/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/pbkdf2"
)

const (
	passwordSaltBytes = 16
	passwordRounds    = 150000
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	digest := pbkdf2.Key([]byte(password), salt, passwordRounds, sha256.Size, sha256.New)
	return hex.EncodeToString(salt) + "$" + hex.EncodeToString(digest), nil
}

func VerifyPassword(password, passwordHash string) bool {
	parts := strings.SplitN(passwordHash, "$", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	derived := pbkdf2.Key([]byte(password), salt, passwordRounds, len(expected), sha256.New)
	return hmac.Equal(expected, derived)
}

func CreateAccessToken(cfg config.Config, subject string) (string, error) {
	return createAccessTokenWithClaims(cfg, jwt.MapClaims{"sub": subject})
}

func CreateAdminAccessToken(cfg config.Config, adminID uint, username string) (string, error) {
	claims := jwt.MapClaims{
		"uid": strconv.FormatUint(uint64(adminID), 10),
	}
	username = strings.TrimSpace(username)
	if username != "" {
		claims["sub"] = username
	}
	return createAccessTokenWithClaims(cfg, claims)
}

func createAccessTokenWithClaims(cfg config.Config, claims jwt.MapClaims) (string, error) {
	if cfg.Algorithm != "" && cfg.Algorithm != "HS256" {
		return "", errors.New("only HS256 JWT signing is supported")
	}
	expiresAt := time.Now().UTC().Add(time.Duration(cfg.AccessTokenExpireMinutes) * time.Minute)
	if claims == nil {
		claims = jwt.MapClaims{}
	}
	claims["exp"] = expiresAt.Unix()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.SecretKey))
}

func DecodeAccessToken(cfg config.Config, tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected JWT signing method")
		}
		return []byte(cfg.SecretKey), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid JWT")
	}
	return claims, nil
}
