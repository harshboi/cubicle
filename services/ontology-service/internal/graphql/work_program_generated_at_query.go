package graphql

import (
	"time"

	entsql "entgo.io/ent/dialect/sql"
)

func workProgramGeneratedAtTextPredicate(field string, generatedAt time.Time) func(*entsql.Selector) {
	values := workProgramGeneratedAtTextValues(generatedAt)
	return func(selector *entsql.Selector) {
		args := make([]any, 0, len(values))
		for _, value := range values {
			args = append(args, value)
		}
		selector.Where(entsql.In(selector.C(field), args...))
	}
}

func workProgramGeneratedAtTextValues(generatedAt time.Time) []string {
	if generatedAt.IsZero() {
		return nil
	}
	seen := map[string]bool{}
	out := []string{}
	appendValue := func(value string) {
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}

	appendValue(generatedAt.Format(time.RFC3339Nano))
	appendValue(generatedAt.Format("2006-01-02 15:04:05.999999999-07:00"))
	utc := generatedAt.UTC()
	appendValue(utc.Format(time.RFC3339Nano))
	appendValue(utc.Format("2006-01-02 15:04:05.999999999-07:00"))
	appendValue(utc.Format("2006-01-02T15:04:05.999999999") + "+00:00")
	return out
}
