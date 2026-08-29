package tui

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	styleJSONKey  = styleKey
	styleJSONStr  = styleOK
	styleJSONNum  = styleWarn
	styleJSONLit  = styleRef.Underline(false)
	styleJSONPunc = styleDim
)

// prettyJSON renders v as indented JSON.
func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

var jsonTokenRe = regexp.MustCompile(`"(?:[^"\\]|\\.)*"\s*:|"(?:[^"\\]|\\.)*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|\btrue\b|\bfalse\b|\bnull\b|[{}\[\],]`)

// highlightJSON adds colour to indented JSON text, line by line.
func highlightJSON(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = jsonTokenRe.ReplaceAllStringFunc(line, func(tok string) string {
			switch {
			case strings.HasSuffix(tok, ":"):
				key := strings.TrimRight(tok[:len(tok)-1], " \t")
				return styleJSONKey.Render(key) + styleJSONPunc.Render(":")
			case strings.HasPrefix(tok, `"`):
				return styleJSONStr.Render(tok)
			case tok == "true" || tok == "false" || tok == "null":
				return styleJSONLit.Render(tok)
			case len(tok) == 1 && strings.ContainsAny(tok, "{}[],"):
				return styleJSONPunc.Render(tok)
			default:
				return styleJSONNum.Render(tok)
			}
		})
	}
	return strings.Join(lines, "\n")
}
