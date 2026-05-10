package registrationpattern

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var (
	registrationSequencePattern = regexp.MustCompile(`\{n(?::0?(\d+))?\}`)
	registrationRandomPattern   = regexp.MustCompile(`\{rand:\[([^\]]+)\]\{(\d+)\}\}`)
)

func Validate(pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return errors.New("邮箱规则不能为空")
	}
	withoutSequence := registrationSequencePattern.ReplaceAllString(pattern, "")
	withoutRandom := registrationRandomPattern.ReplaceAllString(withoutSequence, "")
	if strings.Contains(withoutRandom, "{rand:") {
		return errors.New("随机规则仅支持 {rand:[字符集]{位数}}，例如 {rand:[0-9]{6}}")
	}
	if strings.Contains(withoutRandom, "{n") {
		return errors.New("邮箱规则仅支持 {n}、{n:03} 和 {rand:[字符集]{位数}}")
	}
	if withoutSequence == pattern && withoutRandom == pattern {
		return errors.New("邮箱规则必须包含 {n} 或 {rand:[字符集]{位数}} 占位符")
	}
	return nil
}

func Format(pattern string, index int) (string, error) {
	if err := Validate(pattern); err != nil {
		return "", err
	}
	value := registrationSequencePattern.ReplaceAllStringFunc(pattern, func(match string) string {
		parts := registrationSequencePattern.FindStringSubmatch(match)
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return strconv.Itoa(index)
		}
		width, err := strconv.Atoi(parts[1])
		if err != nil || width <= 0 {
			return strconv.Itoa(index)
		}
		return fmt.Sprintf("%0*d", width, index)
	})

	var firstErr error
	value = registrationRandomPattern.ReplaceAllStringFunc(value, func(match string) string {
		if firstErr != nil {
			return match
		}
		parts := registrationRandomPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			firstErr = errors.New("随机规则格式错误")
			return match
		}
		charset, err := expandRegistrationRandomCharset(parts[1])
		if err != nil {
			firstErr = err
			return match
		}
		length, err := strconv.Atoi(parts[2])
		if err != nil || length < 1 || length > 64 {
			firstErr = errors.New("随机规则位数必须在 1 到 64 之间")
			return match
		}
		randomValue, err := secureRandomString(charset, length)
		if err != nil {
			firstErr = err
			return match
		}
		return randomValue
	})
	if firstErr != nil {
		return "", firstErr
	}
	return strings.TrimSpace(value), nil
}

func expandRegistrationRandomCharset(pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", errors.New("随机规则字符集不能为空")
	}
	var builder strings.Builder
	for i := 0; i < len(pattern); i++ {
		if i+2 < len(pattern) && pattern[i+1] == '-' {
			start := pattern[i]
			end := pattern[i+2]
			if !validRegistrationRandomRune(start) || !validRegistrationRandomRune(end) || start > end {
				return "", errors.New("随机规则字符集仅支持 0-9、a-z、A-Z 和普通字母数字")
			}
			for ch := start; ch <= end; ch++ {
				builder.WriteByte(ch)
			}
			i += 2
			continue
		}
		if !validRegistrationRandomRune(pattern[i]) {
			return "", errors.New("随机规则字符集仅支持 0-9、a-z、A-Z 和普通字母数字")
		}
		builder.WriteByte(pattern[i])
	}
	charset := dedupeStringBytes(builder.String())
	if charset == "" {
		return "", errors.New("随机规则字符集不能为空")
	}
	return charset, nil
}

func validRegistrationRandomRune(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func dedupeStringBytes(value string) string {
	seen := map[byte]bool{}
	var builder strings.Builder
	for i := 0; i < len(value); i++ {
		if seen[value[i]] {
			continue
		}
		seen[value[i]] = true
		builder.WriteByte(value[i])
	}
	return builder.String()
}

func secureRandomString(charset string, length int) (string, error) {
	if charset == "" {
		return "", errors.New("随机规则字符集不能为空")
	}
	var builder strings.Builder
	max := big.NewInt(int64(len(charset)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		builder.WriteByte(charset[n.Int64()])
	}
	return builder.String(), nil
}
