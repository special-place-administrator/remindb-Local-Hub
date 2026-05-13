package store

import (
	"testing"
	"unicode/utf8"
)

func FuzzRewriteQuery(f *testing.F) {
	seeds := []string{
		"",
		"hello",
		"hello world",
		"snapshot OR tests",
		"snapshot AND tests",
		"snapshot NOT mock",
		`"exact phrase"`,
		"label:snapshot",
		"snap*",
		"NEAR(a b)",
		"(three-tier)",
		"three-tier*",
		"NEAR(three-tier stack)",
		"NEAR(three-tier stack, 5)",
		"file:foo-bar",
		"label:three-tier",
		"content:(three-tier stack)",
		"LR-2026-05-09-001a",
		"three-tier",
		"docker-compose feature",
		"path/to/file",
		"node.id",
		"hello-world goodbye-world",
		"LR-2026-05-09-001a OR three-tier",
		"docker-compose AND configuration",
		"label:snapshot LR-2026-05-09-001a",
		`"quoted phrase" LR-2026-05-09-001a`,
		"naïve",
		"((((",
		")))))",
		"label:",
		`"unbalanced`,
		"NEAR(",
		"AND",
		"OR",
		"NOT",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, q string) {
		if !utf8.ValidString(q) {
			t.Skip()
		}
		_ = rewriteQuery(q)
	})
}
