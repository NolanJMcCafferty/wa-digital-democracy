package main

import (
	"regexp"
	"strings"
)

var (
	nonNameChars       = regexp.MustCompile(`[^\p{L}0-9\s]`)
	generationalSuffix = map[string]bool{
		"jr": true, "sr": true, "ii": true, "iii": true, "iv": true, "v": true,
	}
	honorificTokens = map[string]bool{
		"mr": true, "mrs": true, "ms": true, "miss": true, "mx": true, "dr": true,
		"prof": true, "rev": true, "hon": true, "senator": true, "sen": true,
		"representative": true, "rep": true, "chair": true, "vice": true,
	}
)

// normalizeSpokenName converts common legislative/CSI person-name shapes to a
// comparable token string. CSI commonly stores "Last, First" while speakers say
// "First Last"; transcripts also include honorifics and punctuation.
func normalizeSpokenName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		last := strings.TrimSpace(parts[0])
		first := strings.TrimSpace(parts[1])
		suffix := ""
		if len(parts) > 2 {
			suffix = strings.TrimSpace(parts[2])
		}
		if first != "" && last != "" {
			s = strings.TrimSpace(first + " " + last + " " + suffix)
		}
	}
	s = strings.ReplaceAll(s, "’", "'")
	s = strings.ReplaceAll(s, "-", " ")
	s = nonNameChars.ReplaceAllString(s, " ")
	fields := strings.Fields(s)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, ".")
		if f == "" || honorificTokens[f] || generationalSuffix[f] {
			continue
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}
