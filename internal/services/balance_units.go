package services

import (
	"regexp"
	"strings"
)

var (
	balanceUnitRangeTextPattern  = regexp.MustCompile(`(?i)(余量|已用)\s+(-?[0-9][0-9,.]*)\s*/\s*(-?[0-9][0-9,.]*)\s+(USD|CNY|RMB|EUR|GBP)\b`)
	balanceUnitAmountTextPattern = regexp.MustCompile(`(?i)(余量|已用)\s+(-?[0-9][0-9,.]*)\s+(USD|CNY|RMB|EUR|GBP)\b`)
	balanceUnitLooseTextPattern  = regexp.MustCompile(`(?i)(-?[0-9][0-9,.]*)\s+(USD|CNY|RMB|EUR|GBP)\b`)
	balanceUnitTextPattern       = regexp.MustCompile(`(?i)\b(USD|CNY|RMB|EUR|GBP)\b`)
)

func NormalizeBalanceUnit(unit string) string {
	value := strings.TrimSpace(unit)
	if value == "" {
		return ""
	}
	compact := strings.ToLower(strings.NewReplacer(" ", "", "\t", "", "_", "", "-", "", ".", "").Replace(value))
	switch compact {
	case "$", "＄", "usd", "us$", "$usd", "usd$", "dollar", "dollars", "usdollar", "usdollars", "美元", "美金":
		return "$"
	case "cny", "rmb", "yuan", "renminbi", "人民币", "元":
		return "¥"
	case "eur", "euro", "euros", "欧元":
		return "€"
	case "gbp", "pound", "pounds", "英镑":
		return "£"
	default:
		return value
	}
}

func BalanceUnitIsSymbol(unit string) bool {
	switch NormalizeBalanceUnit(unit) {
	case "$", "¥", "€", "£":
		return true
	default:
		return false
	}
}

func NormalizeBalanceUnitText(value string) string {
	text := strings.TrimSpace(value)
	text = balanceUnitRangeTextPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := balanceUnitRangeTextPattern.FindStringSubmatch(match)
		if len(parts) != 5 {
			return match
		}
		unit := NormalizeBalanceUnit(parts[4])
		return parts[1] + " " + unit + parts[2] + " / " + unit + parts[3]
	})
	text = balanceUnitAmountTextPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := balanceUnitAmountTextPattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		unit := NormalizeBalanceUnit(parts[3])
		return parts[1] + " " + unit + parts[2]
	})
	text = balanceUnitLooseTextPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := balanceUnitLooseTextPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		unit := NormalizeBalanceUnit(parts[2])
		return unit + parts[1]
	})
	return balanceUnitTextPattern.ReplaceAllStringFunc(text, func(match string) string {
		return NormalizeBalanceUnit(match)
	})
}
