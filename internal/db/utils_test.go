package db

import (
	"errors"
	"strings"
)

type sqlCustomQueryMatcher func(query string) error
type sqlMockFifoQueryMatcher struct {
	matchers []sqlCustomQueryMatcher
}

func NewSqlMockFifoQueryMatcher(matchers ...sqlCustomQueryMatcher) *sqlMockFifoQueryMatcher {
	return &sqlMockFifoQueryMatcher{matchers: matchers}
}

func (q *sqlMockFifoQueryMatcher) Match(_, query string) error {
	matcher := q.pop()
	if matcher == nil {
		return errors.New("unexpected call to the sql query matched")
	}
	return matcher(strings.ToLower(query))
}

func (q *sqlMockFifoQueryMatcher) pop() sqlCustomQueryMatcher {
	if len(q.matchers) == 0 {
		return nil
	} else {
		matcher := (q.matchers)[0]
		q.matchers = (q.matchers)[1:]
		return matcher
	}
}
