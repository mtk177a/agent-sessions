package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/safeio"
)

const (
	SchemaVersion = "v0alpha1"
	MaxBytes      = 1 << 20
	MaxDepth      = 64
)

type Config struct {
	SchemaVersion string   `json:"schema_version"`
	Sources       []Source `json:"sources"`
}

type Source struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Root     string `json:"root"`
}

type Defaults interface {
	EnvironmentRoot() (string, bool)
	DefaultRoot() (string, bool)
}

func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "agent-sessions", "config.json"), nil
}

func Load(path string, explicit bool) (Config, error) {
	var result Config
	err := safeio.DecodeJSONFile(path, MaxBytes, MaxDepth, &result)
	if errors.Is(err, os.ErrNotExist) && !explicit {
		return Config{SchemaVersion: SchemaVersion, Sources: []Source{}}, nil
	}
	if err != nil {
		return Config{}, err
	}
	if result.SchemaVersion != SchemaVersion {
		return Config{}, errors.New("unsupported configuration schema version")
	}
	seen := map[string]struct{}{}
	for _, source := range result.Sources {
		if !contract.ValidIdentifier(source.Provider) || !contract.ValidIdentifier(source.ID) {
			return Config{}, errors.New("invalid provider or source instance identifier")
		}
		if !filepath.IsAbs(source.Root) {
			return Config{}, errors.New("source root must be absolute")
		}
		key := source.Provider + "\x00" + source.ID
		if _, ok := seen[key]; ok {
			return Config{}, fmt.Errorf("duplicate source instance for provider %q", source.Provider)
		}
		seen[key] = struct{}{}
	}
	return result, nil
}

func ResolveOne(provider, instance, override string, configured Config, defaults Defaults) (Source, error) {
	if override != "" {
		if !filepath.IsAbs(override) {
			return Source{}, errors.New("source root override must be absolute")
		}
		return Source{ID: instance, Provider: provider, Root: override}, nil
	}
	providerConfigured := false
	for _, source := range configured.Sources {
		if source.Provider == provider {
			providerConfigured = true
			if source.ID == instance {
				return source, nil
			}
		}
	}
	if providerConfigured {
		return Source{}, errors.New("named source instance is not configured")
	}
	if root, ok := defaults.EnvironmentRoot(); ok {
		return Source{ID: instance, Provider: provider, Root: root}, nil
	}
	if root, ok := defaults.DefaultRoot(); ok {
		return Source{ID: instance, Provider: provider, Root: root}, nil
	}
	return Source{}, errors.New("source instance cannot be resolved")
}

func ResolveAll(provider, override, overrideInstance string, configured Config, defaults Defaults) ([]Source, error) {
	if override != "" {
		if overrideInstance == "" {
			overrideInstance = provider + "-default"
		}
		one, err := ResolveOne(provider, overrideInstance, override, Config{}, defaults)
		if err != nil {
			return nil, err
		}
		return []Source{one}, nil
	}
	var matched []Source
	providerConfigured := false
	for _, source := range configured.Sources {
		if source.Provider == provider {
			providerConfigured = true
			if overrideInstance == "" || source.ID == overrideInstance {
				matched = append(matched, source)
			}
		}
	}
	if len(matched) > 0 {
		return matched, nil
	}
	if providerConfigured {
		return nil, errors.New("named source instance is not configured")
	}
	instance := overrideInstance
	if instance == "" {
		instance = provider + "-default"
	}
	one, err := ResolveOne(provider, instance, "", Config{}, defaults)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(one.Root); errors.Is(err, os.ErrNotExist) {
		return []Source{}, nil
	}
	return []Source{one}, nil
}
