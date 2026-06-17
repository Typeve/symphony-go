package commandline

import (
	"fmt"
	"strings"
	"unicode"
)

// Split splits a configured command into an executable name and arguments.
// It intentionally does not implement shell evaluation.
func Split(value, defaultCommand string) (string, []string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultCommand, nil, nil
	}

	fields, err := fields(value)
	if err != nil {
		return "", nil, err
	}
	if len(fields) == 0 {
		return defaultCommand, nil, nil
	}
	return fields[0], fields[1:], nil
}

func fields(value string) ([]string, error) {
	var out []string
	var b strings.Builder
	var quote rune
	inToken := false
	runes := []rune(value)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote == 0 && unicode.IsSpace(r) {
			if inToken {
				out = append(out, b.String())
				b.Reset()
				inToken = false
			}
			continue
		}

		inToken = true
		switch r {
		case '\'', '"':
			if quote == 0 {
				quote = r
				continue
			}
			if quote == r {
				quote = 0
				continue
			}
			b.WriteRune(r)
		case '\\':
			if i+1 >= len(runes) {
				b.WriteRune(r)
				continue
			}
			next := runes[i+1]
			if quote == '\'' {
				b.WriteRune(r)
				continue
			}
			if quote == '"' {
				if next == '"' || next == '\\' {
					b.WriteRune(next)
					i++
				} else {
					b.WriteRune(r)
				}
				continue
			}
			if unicode.IsSpace(next) || next == '\'' || next == '"' || next == '\\' {
				b.WriteRune(next)
				i++
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}

	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in command")
	}
	if inToken {
		out = append(out, b.String())
	}
	return out, nil
}
