// Package pinyin produces searchable pinyin forms for Chinese names.
package pinyin

import (
	"strings"
	"unicode"

	gopinyin "github.com/mozillazg/go-pinyin"
)

// Forms returns the concatenated lowercase full pinyin and the initials.
// ASCII segments (codes such as "HD-01") are preserved so they stay searchable.
func Forms(s string) (full string, initials string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	args := gopinyin.NewArgs()
	args.Style = gopinyin.Normal

	var fullB, initB strings.Builder
	// Walk rune by rune: Han characters go through the library, everything else
	// is lowercased verbatim. The library returns nothing for pure ASCII input,
	// so it cannot be applied to the whole string blindly.
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			parts := gopinyin.Pinyin(string(r), args)
			if len(parts) > 0 && len(parts[0]) > 0 {
				p := strings.ToLower(parts[0][0])
				fullB.WriteString(p)
				initB.WriteString(string([]rune(p)[0]))
				continue
			}
		}
		if unicode.IsSpace(r) || r == '-' || r == '_' || r == '/' {
			continue
		}
		lr := unicode.ToLower(r)
		fullB.WriteRune(lr)
		initB.WriteRune(lr)
	}
	return fullB.String(), initB.String()
}
