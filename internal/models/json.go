package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type JSONMap map[string]any

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (m *JSONMap) Scan(value any) error {
	if value == nil {
		*m = JSONMap{}
		return nil
	}
	var data []byte
	switch typed := value.(type) {
	case []byte:
		data = typed
	case string:
		data = []byte(typed)
	default:
		return fmt.Errorf("unsupported JSONMap scan type %T", value)
	}
	if len(data) == 0 {
		*m = JSONMap{}
		return nil
	}
	return json.Unmarshal(data, m)
}
