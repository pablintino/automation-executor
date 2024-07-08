package db

import "database/sql/driver"

type AnyString struct{}

func (a AnyString) Match(v driver.Value) bool {
	_, ok := v.(string)
	return ok
}
