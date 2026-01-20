package sqlutil

import (
	"strconv"
	"strings"
)

// ReplacePlaceholders converts '?' placeholders to $1, $2, ... for PostgreSQL.
func ReplacePlaceholders(query string) string {
	if !strings.Contains(query, "?") {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	index := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(index))
			index++
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}
