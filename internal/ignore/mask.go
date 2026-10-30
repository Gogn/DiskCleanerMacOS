// Package ignore implements the restricted BSD du -I name pattern syntax.
package ignore

import (
	"fmt"
	"path"
	"strings"
)

func Validate(mask string) error {
	if mask == "" || strings.ContainsAny(mask, "/\x00") {
		return fmt.Errorf("mask must be a nonempty name without / or NUL")
	}
	escaped := false
	for _, r := range mask {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
		} else if r == '[' || r == ']' {
			return fmt.Errorf("escape literal brackets in masks")
		}
	}
	if escaped {
		return fmt.Errorf("unfinished mask escape")
	}
	return nil
}
func Match(mask, name string) bool {
	if Validate(mask) != nil {
		return false
	}
	ok, _ := path.Match(mask, name)
	return ok
}
func Literal(name string) string {
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(`\*?[]`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
func Any(masks []string, name string) bool {
	for _, m := range masks {
		if Match(m, name) {
			return true
		}
	}
	return false
}
