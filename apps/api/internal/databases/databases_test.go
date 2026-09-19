package databases

import "testing"

func TestSlugifyIdent(t *testing.T) {
	cases := map[string]string{
		"My DB":     "my_db",
		"123start":  "db_123start",
		"":          "db",
		"hello!!!":  "hello",
		"a":         "a",
	}
	for in, want := range cases {
		if got := slugifyIdent(in); got != want {
			t.Fatalf("slugifyIdent(%q)=%q want %q", in, got, want)
		}
	}
}
