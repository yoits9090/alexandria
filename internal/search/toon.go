package search

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// EncodeTOON emits the compact, LLM-facing search projection. Results are a
// fixed-shape table so optional provider metadata never forces TOON list form;
// row IDs are explicit and stable for citations.
type TOONResponse struct {
	Query   string
	Results []TOONResult
}
type TOONResult struct {
	ID      int
	Title   string
	URL     string
	Snippet string
	Source  string
}
type toonColumn struct {
	name string
	get  func(TOONResult) string
}

func EncodeTOON(resp SearchResponse) (string, error) {
	var b strings.Builder
	query, err := toonString(resp.Query, ',')
	if err != nil {
		return "", fmt.Errorf("encode TOON query: %w", err)
	}
	b.WriteString("query: ")
	b.WriteString(query)
	b.WriteByte('\n')
	rows := make([]TOONResult, len(resp.Results))
	for i, r := range resp.Results {
		rows[i] = TOONResult{ID: i + 1, Title: r.Title, URL: r.URL, Snippet: r.Snippet, Source: r.Source}
	}
	if len(rows) == 0 {
		b.WriteString("sources: []")
		return b.String(), nil
	}
	b.WriteString("sources[")
	b.WriteString(strconv.Itoa(len(rows)))
	b.WriteString("]{id,title,url,snippet,source}:")
	columns := []toonColumn{{"title", func(r TOONResult) string { return r.Title }}, {"url", func(r TOONResult) string { return r.URL }}, {"snippet", func(r TOONResult) string { return r.Snippet }}, {"source", func(r TOONResult) string { return r.Source }}}
	for _, row := range rows {
		b.WriteString("\n  ")
		b.WriteString(strconv.Itoa(row.ID))
		for _, column := range columns {
			value, e := toonString(column.get(row), ',')
			if e != nil {
				return "", fmt.Errorf("encode TOON result: %w", e)
			}
			b.WriteByte(',')
			b.WriteString(value)
		}
	}
	return b.String(), nil
}

func toonString(value string, delimiter byte) (string, error) {
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("string is not valid UTF-8")
	}
	if !toonNeedsQuote(value, delimiter) {
		return value, nil
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, "\\u%04x", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}
func toonNeedsQuote(value string, delimiter byte) bool {
	if value == "" || value == "true" || value == "false" || value == "null" || value[0] == '-' || value[0] == '#' || value[0] == ' ' || value[0] == '\t' || value[len(value)-1] == ' ' || value[len(value)-1] == '\t' || numericLike(value) {
		return true
	}
	for _, r := range value {
		if r == rune(delimiter) || r == ':' || r == '"' || r == '\\' || r == '[' || r == ']' || r == '{' || r == '}' || r < 0x20 {
			return true
		}
	}
	return false
}
func numericLike(value string) bool {
	i := 0
	if i < len(value) && (value[i] == '+' || value[i] == '-') {
		i++
	}
	start := i
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		i++
	}
	if i == start {
		return false
	}
	if i < len(value) && value[i] == '.' {
		i++
		fraction := i
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
		}
		if i == fraction {
			return false
		}
	}
	if i < len(value) && (value[i] == 'e' || value[i] == 'E') {
		i++
		if i < len(value) && (value[i] == '+' || value[i] == '-') {
			i++
		}
		exponent := i
		for i < len(value) && value[i] >= '0' && value[i] <= '9' {
			i++
		}
		if i == exponent {
			return false
		}
	}
	return i == len(value)
}
