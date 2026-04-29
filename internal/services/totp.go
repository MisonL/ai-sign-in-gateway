package services

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TOTPConfig struct {
	Secret    string
	Digits    int
	Period    int
	Algorithm string
}

func ParseTOTPConfig(secret, otpauthURL string) (*TOTPConfig, error) {
	if direct := strings.TrimSpace(secret); direct != "" {
		return &TOTPConfig{Secret: direct, Digits: 6, Period: 30, Algorithm: "SHA1"}, nil
	}
	otpauth := strings.TrimSpace(otpauthURL)
	if otpauth == "" {
		return nil, nil
	}
	parsed, err := url.Parse(otpauth)
	if err != nil || parsed.Scheme != "otpauth" {
		return nil, errors.New("otpauth 链接格式不正确")
	}
	query := parsed.Query()
	parsedSecret := strings.TrimSpace(query.Get("secret"))
	if parsedSecret == "" {
		return nil, errors.New("otpauth 链接里缺少 secret")
	}
	digits, err := strconv.Atoi(stringWithDefault(query.Get("digits"), "6"))
	if err != nil || digits <= 0 {
		return nil, errors.New("otpauth 链接 digits 无法解析")
	}
	period, err := strconv.Atoi(stringWithDefault(query.Get("period"), "30"))
	if err != nil || period <= 0 {
		return nil, errors.New("otpauth 链接 period 无法解析")
	}
	algorithm := strings.ToUpper(strings.TrimSpace(query.Get("algorithm")))
	if algorithm == "" {
		algorithm = "SHA1"
	}
	return &TOTPConfig{Secret: parsedSecret, Digits: digits, Period: period, Algorithm: algorithm}, nil
}

func GenerateTOTPCode(config TOTPConfig, forTime ...int64) (string, int, error) {
	now := time.Now().Unix()
	if len(forTime) > 0 {
		now = forTime[0]
	}
	counter := now / int64(config.Period)
	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, uint64(counter))
	secretBytes, err := normalizeBase32Secret(config.Secret)
	if err != nil {
		return "", 0, err
	}
	mac := hmac.New(hashFor(config.Algorithm), secretBytes)
	mac.Write(counterBytes)
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 0x0F
	if int(offset)+4 > len(digest) {
		return "", 0, errors.New("TOTP digest 长度不足")
	}
	codeInt := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7FFFFFFF
	mod := uint32(1)
	for i := 0; i < config.Digits; i++ {
		mod *= 10
	}
	code := fmt.Sprintf("%0*d", config.Digits, codeInt%mod)
	expiresIn := config.Period - int(now%int64(config.Period))
	return code, expiresIn, nil
}

func ResolveTOTPCode(secret, otpauthURL string) (string, int, error) {
	config, err := ParseTOTPConfig(secret, otpauthURL)
	if err != nil {
		return "", 0, err
	}
	if config == nil {
		return "", 0, nil
	}
	return GenerateTOTPCode(*config)
}

func hashFor(algorithm string) func() hash.Hash {
	switch strings.ToUpper(strings.TrimSpace(algorithm)) {
	case "SHA256":
		return sha256.New
	case "SHA512":
		return sha512.New
	default:
		return sha1.New
	}
}

func normalizeBase32Secret(value string) ([]byte, error) {
	cleaned := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	if cleaned == "" {
		return nil, errors.New("缺少 TOTP secret")
	}
	if pad := len(cleaned) % 8; pad != 0 {
		cleaned += strings.Repeat("=", 8-pad)
	}
	return base32.StdEncoding.DecodeString(cleaned)
}

func stringWithDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
