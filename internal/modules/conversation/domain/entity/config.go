package entity

import (
	"encoding/json"
	"fmt"
	"os"

	permentity "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/entity"
)

// LoadConfig loads configuration from a file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &config, nil
}

// ApplyOverrides applies CLI flag overrides to the config.
func (c *Config) ApplyOverrides(model string, permMode string) {
	if model != "" {
		c.Model = model
	}
	if permMode != "" {
		c.PermissionMode = permentity.PermissionMode(permMode)
	}
}
