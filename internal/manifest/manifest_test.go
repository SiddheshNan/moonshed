package manifest

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	m, err := Parse([]byte("run: python app.py\nport: 9988\nhealth: /\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Port != 9988 || m.Health == nil || m.Health.HTTP != "/" {
		t.Fatalf("got port %d health %+v", m.Port, m.Health)
	}

	m, err = Parse([]byte("run: x\nhealth:\n  tcp: true\n  interval: 10s\n  failures: 5\n  grace: 1m\n"))
	if err != nil || !m.Health.TCP || m.Health.Interval.D() != 10*time.Second || m.Health.Failures != 5 || m.Health.Grace.D() != time.Minute {
		t.Fatalf("long form: %+v %v", m.Health, err)
	}

	m, err = Parse([]byte("build:\n  folder: ./api\n  dockerfile: docker/Dockerfile.prod\n  stage: runtime\n  args:\n    NODE_ENV: production\n"))
	if err != nil || m.Build.Folder != "api" || m.Build.DockerfilePath() != "docker/Dockerfile.prod" || m.Build.Args["NODE_ENV"] != "production" {
		t.Fatalf("build: %+v %v", m.Build, err)
	}

	for _, bad := range []string{"run: x\nprot: 80\n", "health:\n  interval: 10s\n", "build:\n  folder: ../up\n", "persist:\n  - /etc\n"} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Fatalf("want an error for %q", bad)
		}
	}
	if _, err := Parse([]byte("run: x\nprot: 80\n")); !strings.Contains(err.Error(), `unknown key "prot"`) {
		t.Fatalf("want unknown key error, got %v", err)
	}
	if _, err := Parse([]byte("")); err != nil {
		t.Fatalf("empty file: %v", err)
	}
}

func TestEffective(t *testing.T) {
	file, _ := Parse([]byte("run: python app.py\nhealth: /healthz\npersist:\n  - data/\n"))
	run, persist := "gunicorn app:app", []string{}
	m := Effective(file, Overrides{Run: &run, Persist: &persist, Health: &Health{}})
	if m.Run.Current() != run || len(m.Persist) != 0 || !m.Health.ProcessOnly() {
		t.Fatalf("overrides not applied: %+v", m)
	}
	if file.Run.Current() != "python app.py" {
		t.Fatal("Effective changed the file")
	}

	c := Effective(file, Overrides{}).Check()
	if c.Interval.D() != DefaultInterval || c.Failures != DefaultFailures || c.Grace.D() != DefaultGrace {
		t.Fatalf("defaults: %+v", c)
	}
	if p := Effective(nil, Overrides{}).Check(); !p.ProcessOnly() || p.Failures != 1 {
		t.Fatalf("process check: %+v", p)
	}
}

func TestDurationJSON(t *testing.T) {
	var h Health
	if err := json.Unmarshal([]byte(`{"interval":"45s","timeout":5000000000}`), &h); err != nil {
		t.Fatal(err)
	}
	if h.Interval.D() != 45*time.Second || h.Timeout.D() != 5*time.Second {
		t.Fatalf("got %+v", h)
	}
	out, _ := json.Marshal(Health{Interval: Duration(time.Minute)})
	if !strings.Contains(string(out), `"interval":"1m0s"`) {
		t.Fatalf("got %s", out)
	}
}
