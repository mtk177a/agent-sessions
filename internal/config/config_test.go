package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRejectsDuplicateAndUnknownSources(t *testing.T) {
	dir := t.TempDir()
	duplicate := filepath.Join(dir, "duplicate.json")
	duplicateData, err := json.Marshal(Config{SchemaVersion: SchemaVersion, Sources: []Source{
		{ID: "demo", Provider: "synthetic", Root: filepath.Join(dir, "a")},
		{ID: "demo", Provider: "synthetic", Root: filepath.Join(dir, "b")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(duplicate, duplicateData, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(duplicate, true); err == nil {
		t.Fatal("duplicate source was accepted")
	}

	unknown := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"schema_version":"v0alpha1","extra":true,"sources":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(unknown, true); err == nil {
		t.Fatal("unknown field was accepted")
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

type fakeDefaults struct {
	environment string
	fallback    string
}

func (f fakeDefaults) EnvironmentRoot() (string, bool) { return f.environment, f.environment != "" }
func (f fakeDefaults) DefaultRoot() (string, bool)     { return f.fallback, f.fallback != "" }
