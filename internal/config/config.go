package config

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultRetentionTTL     = 30 * 24 * time.Hour
	defaultArchivalPrefix   = "kmortem/nodereports/"
	defaultArchivalProvider = "s3"
)

// RetentionConfig holds TTL configuration for NodeReports.
type RetentionConfig struct {
	// TTL is how long NodeReport objects live in the cluster.
	// Measured from NodeReport creation timestamp.
	TTL time.Duration
}

// ArchivalConfig holds S3 archival configuration.
type ArchivalConfig struct {
	// Enabled controls whether archival is active.
	Enabled bool
	// Provider is the archival backend (currently only "s3").
	Provider string
	// Bucket is the S3 bucket name.
	Bucket string
	// Prefix is the S3 key prefix.
	Prefix string
	// Region is the AWS region.
	Region string
}

// Config is the top-level operator configuration.
type Config struct {
	Retention RetentionConfig
	Archival  ArchivalConfig
}

// Default returns a Config populated with default values.
func Default() *Config {
	return &Config{
		Retention: RetentionConfig{
			TTL: defaultRetentionTTL,
		},
		Archival: ArchivalConfig{
			Enabled:  false,
			Provider: defaultArchivalProvider,
			Prefix:   defaultArchivalPrefix,
		},
	}
}

// LoadFromEnv loads configuration from environment variables, falling back to defaults.
//
// Supported environment variables:
//
//	KMORTEM_RETENTION_TTL          - duration string, e.g. "720h" (default: 720h / 30 days)
//	KMORTEM_ARCHIVAL_ENABLED       - "true" or "false" (default: false)
//	KMORTEM_ARCHIVAL_BUCKET        - S3 bucket name
//	KMORTEM_ARCHIVAL_PREFIX        - S3 key prefix (default: kmortem/nodereports/)
//	KMORTEM_ARCHIVAL_REGION        - AWS region
func LoadFromEnv() *Config {
	cfg := Default()

	if v := os.Getenv("KMORTEM_RETENTION_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Retention.TTL = d
		}
	}

	if v := os.Getenv("KMORTEM_ARCHIVAL_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Archival.Enabled = b
		}
	}

	if v := os.Getenv("KMORTEM_ARCHIVAL_BUCKET"); v != "" {
		cfg.Archival.Bucket = v
	}

	if v := os.Getenv("KMORTEM_ARCHIVAL_PREFIX"); v != "" {
		cfg.Archival.Prefix = v
	}

	if v := os.Getenv("KMORTEM_ARCHIVAL_REGION"); v != "" {
		cfg.Archival.Region = v
	}

	return cfg
}
