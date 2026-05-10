package plugins

import "fmt"

func jsonStringForTest(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
