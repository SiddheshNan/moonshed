package manifest

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	m, err := Parse([]byte("run: python app.py\nport: 9988\nhealth: /\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Port != 9988 || m.Health == nil || m.Health.HTTP != "/" || m.Health.Interval == 0 {
		t.Fatalf("got port %d health %+v", m.Port, m.Health)
	}

	m, err = Parse([]byte("run: x\nhealth:\n  tcp: true\n  interval: 10s\n"))
	if err != nil || !m.Health.TCP || m.Health.Interval.Seconds() != 10 {
		t.Fatalf("long form: %+v %v", m, err)
	}

	_, err = Parse([]byte("run: x\nprot: 80\n"))
	if err == nil || !strings.Contains(err.Error(), `unknown key "prot"`) {
		t.Fatalf("want unknown key error, got %v", err)
	}

	if _, err := Parse([]byte("")); err != nil {
		t.Fatalf("empty file: %v", err)
	}
}
