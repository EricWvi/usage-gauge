// Package config resolves runtime paths and loads endpoints.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"usage-gauge/internal/types"
)

// Dir resolves the config directory from CONFIG_DIR, defaulting to ./config.
func Dir() string {
	if v := os.Getenv("CONFIG_DIR"); v != "" {
		return v
	}
	return "./config"
}

// EndpointsPath returns the path to endpoints.yaml inside the config dir.
func EndpointsPath() string {
	return filepath.Join(Dir(), "endpoints.yaml")
}

// DBPath returns the path to gauge.db inside the config dir.
func DBPath() string {
	return filepath.Join(Dir(), "gauge.db")
}

// ParserPath returns the path to a parser file inside the config dir.
func ParserPath(name string) string {
	return filepath.Join(Dir(), "parser", name+".js")
}

// LoadEndpoints reads and parses endpoints.yaml.
func LoadEndpoints() ([]types.EndpointConfig, error) {
	data, err := os.ReadFile(EndpointsPath())
	if err != nil {
		return nil, fmt.Errorf("read endpoints.yaml: %w", err)
	}
	var f types.EndpointsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse endpoints.yaml: %w", err)
	}
	return f.Endpoints, nil
}
