package models

import (
	"encoding/json"
	"errors"
	"github.com/lib/pq"
)

type StringMap map[string]interface{}
type StringArray *pq.StringArray

func (m *StringMap) Scan(value any) error {
	if value == nil {
		return nil
	}

	buf, ok := value.([]byte)
	if ok {
		return json.Unmarshal(buf, m)
	}

	str, ok := value.(string)
	if ok {
		return json.Unmarshal([]byte(str), m)
	}

	return errors.New("cannot decode string map from types that are not byte/string")
}
