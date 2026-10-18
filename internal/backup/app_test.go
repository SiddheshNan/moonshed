package backup

import (
	"maps"
	"testing"
)

func TestEnvRoundTrip(t *testing.T) {
	env := map[string]string{
		"PLAIN":   "value",
		"SPACES":  "hello world",
		"QUOTES":  `say "hi"`,
		"LINES":   "a\nb",
		"DOLLAR":  "$HOME/x",
		"EMPTY":   "",
		"EQUALS":  "a=b",
		"BACKSLA": `C:\path`,
	}
	got := parseEnv(string(formatEnv(env)))
	if !maps.Equal(got, env) {
		t.Fatalf("round trip:\n got %q\nwant %q", got, env)
	}
	hand := parseEnv("# comment\nexport A=1\nB='single quoted'\n\nC = spaced\n")
	if hand["A"] != "1" || hand["B"] != "single quoted" || hand["C"] != " spaced" {
		t.Fatalf("hand-written: %q", hand)
	}
}

func TestUnder(t *testing.T) {
	persist := []string{"data/", "config.ini", "storage/uploads/"}
	for p, want := range map[string]bool{
		"data": true, "data/db.sqlite": true, "config.ini": true, "storage/uploads/a.png": true,
		"storage/other.txt": false, "database.py": false, "app.py": false,
	} {
		if under(p, persist) != want {
			t.Errorf("under(%q) = %v, want %v", p, !want, want)
		}
	}
}
