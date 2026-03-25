package config

import (
	"os"
	"time"
)

const (
	defaultRetentionTTL = 30 * 24 * time.Hour
)

// RetentionConfig holds TTL configuration for NodeReports.
type RetentionConfig struct {
	// TTL is how long NodeReport objects live in the cluster.
	// Measured from NodeReport creation timestamp.
	TTL time.Duration
}

// Config is the top-level operator configuration.
type Config struct {
	Retention RetentionConfig
}

// Default returns a Config populated with default values.
func Default() *Config {
	return &Config{
		Retention: RetentionConfig{
			TTL: defaultRetentionTTL,
		},
	}
}

// LoadFromEnv loads configuration from environment variables, falling back to defaults.
//
// Supported environment variables:
//
//	KMORTEM_RETENTION_TTL - duration string, e.g. "720h" (default: 720h / 30 days)
func LoadFromEnv() *Config {
	cfg := Default()

	if v := os.Getenv("KMORTEM_RETENTION_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Retention.TTL = d
		}
	}

	return cfg
}
