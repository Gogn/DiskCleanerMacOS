package ignore

import "testing"

func TestMaskParity(t *testing.T) {
	for _, m := range []string{"", "a/b", "[a]", `abc\`} {
		if Validate(m) == nil {
			t.Errorf("accepted %q", m)
		}
	}
	for _, n := range []string{"cache", ".cache", "a*b?[]\\", "é\n"} {
		if !Match(Literal(n), n) {
			t.Errorf("literal %q", n)
		}
	}
	for _, m := range []string{"cache", "c*", "?ache"} {
		if !Match(m, "cache") || Match(m, "other") {
			t.Error(m)
		}
	}
}
