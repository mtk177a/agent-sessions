package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAcceptsConfigWithoutSources(t *testing.T) {
	for _, data := range []string{`{}`, `{"sources":[]}`, `{"sources":null}`} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			configured, err := Load(path, true)
			if err != nil || len(configured.Sources) != 0 {
				t.Fatalf("Load() = %#v, %v; want empty sources", configured, err)
			}
		})
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	root := t.TempDir()
	source := fmt.Sprintf(`{"id":"demo","provider":"synthetic","root":%q}`, root)
	for _, tc := range []struct {
		name    string
		data    string
		wantErr string
	}{
		{"legacy schema version", `{"schema_version":"v1","sources":[]}`, `unknown field "schema_version"`},
		{"unknown configuration field", `{"sources":[],"extra":true}`, `unknown field "extra"`},
		{"unknown source field", `{"sources":[{"extra":true}]}`, `unknown field "extra"`},
		{"null configuration", `null`, "configuration must be a JSON object"},
		{"non-array sources", `{"sources":{}}`, "cannot unmarshal object"},
		{"missing ID", fmt.Sprintf(`{"sources":[{"provider":"synthetic","root":%q}]}`, root), "invalid provider or source instance identifier"},
		{"invalid provider", fmt.Sprintf(`{"sources":[{"id":"demo","provider":"Synthetic","root":%q}]}`, root), "invalid provider or source instance identifier"},
		{"relative root", `{"sources":[{"id":"demo","provider":"synthetic","root":"relative"}]}`, "source root must be absolute"},
		{"duplicate source", `{"sources":[` + source + `,` + source + `]}`, "duplicate source instance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path, true); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Load() error = %v; want %q", err, tc.wantErr)
			}
		})
	}
}

func TestAbsentDefaultConfigDoesNotCreateState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.json")
	configured, err := Load(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(configured.Sources) != 0 {
		t.Fatalf("unexpected sources: %#v", configured.Sources)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("default config read created state: %v", err)
	}
}

func TestResolvePrecedence(t *testing.T) {
	base := t.TempDir()
	cliRoot := filepath.Join(base, "cli")
	configRoot := filepath.Join(base, "config")
	environmentRoot := filepath.Join(base, "environment")
	defaultRoot := filepath.Join(base, "default")
	configured := Config{Sources: []Source{{ID: "demo", Provider: "synthetic", Root: configRoot}}}
	adapter := fakeDefaults{environment: environmentRoot, fallback: defaultRoot}

	cases := []struct {
		name     string
		override string
		config   Config
		want     string
	}{
		{"cli", cliRoot, configured, cliRoot},
		{"config", "", configured, configRoot},
		{"environment", "", Config{}, environmentRoot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveOne("synthetic", "demo", tc.override, tc.config, adapter)
			if err != nil {
				t.Fatal(err)
			}
			if got.Root != tc.want {
				t.Fatalf("root = %q, want %q", got.Root, tc.want)
			}
		})
	}
	got, err := ResolveOne("synthetic", "demo", "", Config{}, fakeDefaults{fallback: defaultRoot})
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != defaultRoot {
		t.Fatalf("default root = %q, want %q", got.Root, defaultRoot)
	}
}

func TestResolveAllSkipsOnlyMissingImplicitRoots(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	for _, defaults := range []fakeDefaults{
		{environment: missing},
		{fallback: missing},
	} {
		got, err := ResolveAll("synthetic", "", "", Config{}, defaults)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("missing implicit root resolved to %#v", got)
		}
	}

	explicit, err := ResolveAll("synthetic", missing, "synthetic-default", Config{}, fakeDefaults{})
	if err != nil || len(explicit) != 1 || explicit[0].Root != missing {
		t.Fatalf("explicit root was skipped: %#v, %v", explicit, err)
	}
	configured, err := ResolveAll("synthetic", "", "", Config{Sources: []Source{{ID: "synthetic-default", Provider: "synthetic", Root: missing}}}, fakeDefaults{})
	if err != nil || len(configured) != 1 || configured[0].Root != missing {
		t.Fatalf("configured root was skipped: %#v, %v", configured, err)
	}
}

func TestResolveAllSkipsProviderWithoutImplicitRoot(t *testing.T) {
	got, err := ResolveAll("explicit-only", "", "", Config{}, fakeDefaults{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("provider without an implicit root resolved to %#v", got)
	}
}

type fakeDefaults struct {
	environment string
	fallback    string
}

func (f fakeDefaults) EnvironmentRoot() (string, bool) { return f.environment, f.environment != "" }
func (f fakeDefaults) DefaultRoot() (string, bool)     { return f.fallback, f.fallback != "" }
