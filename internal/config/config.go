package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the main configuration structure
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Generator GeneratorConfig `yaml:"generator"`
	Users     []UserConfig    `yaml:"users"`
	Topics    []TopicConfig   `yaml:"topics"`
	Scenarios ScenariosConfig `yaml:"scenarios"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Output    OutputConfig    `yaml:"output"`
	Audit     AuditConfig     `yaml:"audit"`
}

// ServerConfig defines Tinode server connection details
type ServerConfig struct {
	URL         string `yaml:"url"`
	APIKey      string `yaml:"api_key"`
	APIEndpoint string `yaml:"api_endpoint"`
	TimeoutSecs int    `yaml:"timeout_seconds"`
}

// GeneratorConfig defines generator-wide settings
type GeneratorConfig struct {
	MaxConcurrency     int    `yaml:"max_concurrency"`
	RateLimitPerSecond int    `yaml:"rate_limit_per_second"`
	DryRun             bool   `yaml:"dry_run"`
	LogLevel           string `yaml:"log_level"`
	LogOutput          string `yaml:"log_output"`
	LogFile            string `yaml:"log_file"`
	EventsLogFile      string `yaml:"events_log_file"`    // Added by CLI
	MetricsAddr        string `yaml:"metrics_addr"`       // Added by CLI
	SelectedScenarios  string `yaml:"selected_scenarios"` // Added by CLI
}

// UserConfig defines a test user
type UserConfig struct {
	ID          string `yaml:"id"`
	Login       string `yaml:"login"`
	Password    string `yaml:"password"`
	Description string `yaml:"description"`
}

// TopicConfig defines a test topic
type TopicConfig struct {
	Name              string `yaml:"name"`
	Type              string `yaml:"type"` // "group" or "p2p"
	Description       string `yaml:"description"`
	DefaultAccessAuth string `yaml:"default_access_auth"`
	DefaultAccessAnon string `yaml:"default_access_anon"`
}

// ScenariosConfig contains all scenario configurations
type ScenariosConfig struct {
	BruteForce      BruteForceConfig         `yaml:"brute_force"`
	ConcurrentSess  ConcurrentSessionsConfig `yaml:"concurrent_sessions"`
	MassDelete      MassDeleteConfig         `yaml:"mass_delete"`
	VolumeAnomaly   VolumeAnomalyConfig      `yaml:"volume_anomaly"`
	Enumeration     EnumerationConfig        `yaml:"enumeration"`
	InactiveAccount InactiveAccountConfig    `yaml:"inactive_account"`
	OffHours        OffHoursConfig           `yaml:"off_hours"`
	Normal          NormalConfig             `yaml:"normal"`
}

// BruteForceConfig - R1 scenario
type BruteForceConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Description string   `yaml:"description"`
	UserID      string   `yaml:"user_id"`
	TargetLogin string   `yaml:"target_login"`
	Attempts    []string `yaml:"password_attempts"`
	MinAttempts int      `yaml:"min_attempts"`
	IntervalMs  int      `yaml:"interval_ms"`
	TimeoutSecs int      `yaml:"timeout_seconds"`
}

// ConcurrentSessionsConfig - R2 scenario
type ConcurrentSessionsConfig struct {
	Enabled            bool   `yaml:"enabled"`
	Description        string `yaml:"description"`
	UserID             string `yaml:"user_id"`
	SessionCount       int    `yaml:"session_count"`
	DurationSecs       int    `yaml:"duration_seconds"`
	MessagesPerSession int    `yaml:"messages_per_session"`
	IntervalMs         int    `yaml:"interval_ms"`
}

// MassDeleteConfig - R3 scenario
type MassDeleteConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Description       string `yaml:"description"`
	UserID            string `yaml:"user_id"`
	PeerUserID        string `yaml:"peer_user_id"`
	PublishCount      int    `yaml:"publish_count"`
	PublishIntervalMs int    `yaml:"publish_interval_ms"`
	DeleteBurstCount  int    `yaml:"delete_burst_count"`
	DeleteIntervalMs  int    `yaml:"delete_interval_ms"`
	TimeoutSecs       int    `yaml:"timeout_seconds"`
}

// VolumeAnomalyConfig - R4 scenario
type VolumeAnomalyConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Description       string `yaml:"description"`
	UserID            string `yaml:"user_id"`
	TopicName         string `yaml:"topic_name"`
	MessagesPerMinute int    `yaml:"messages_per_minute"`
	DurationSecs      int    `yaml:"duration_seconds"`
	MessageContent    string `yaml:"message_content"`
}

// EnumerationConfig - R5 scenario
type EnumerationConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Description string   `yaml:"description"`
	UserID      string   `yaml:"user_id"`
	TopicIDs    []string `yaml:"topic_ids"`
	IntervalMs  int      `yaml:"interval_ms"`
	TimeoutSecs int      `yaml:"timeout_seconds"`
}

// InactiveAccountConfig - R6 scenario
type InactiveAccountConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Description       string `yaml:"description"`
	UserID            string `yaml:"user_id"`
	TopicName         string `yaml:"topic_name"`
	MessageBurstCount int    `yaml:"message_burst_count"`
	IntervalMs        int    `yaml:"interval_ms"`
	TimeoutSecs       int    `yaml:"timeout_seconds"`
}

// OffHoursConfig - R7 scenario
type OffHoursConfig struct {
	Enabled            bool   `yaml:"enabled"`
	Description        string `yaml:"description"`
	UserID             string `yaml:"user_id"`
	TopicName          string `yaml:"topic_name"`
	MessagesPerMinute  int    `yaml:"messages_per_minute"`
	DurationSecs       int    `yaml:"duration_seconds"`
	BusinessHoursStart string `yaml:"business_hours_start"`
	BusinessHoursEnd   string `yaml:"business_hours_end"`
	Timezone           string `yaml:"timezone"`
	SimulateTime       string `yaml:"simulate_time"`
}

// NormalConfig - baseline normal traffic
type NormalConfig struct {
	Enabled           bool     `yaml:"enabled"`
	Description       string   `yaml:"description"`
	UserIDs           []string `yaml:"user_ids"`
	MessagesPerMinute int      `yaml:"messages_per_minute"`
	DurationSecs      int      `yaml:"duration_seconds"`
	P2PMessagesRatio  float64  `yaml:"p2p_messages_ratio"`
	MessageTemplates  []string `yaml:"message_templates"`
}

// MetricsConfig defines metrics collection
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// OutputConfig defines output/reporting
type OutputConfig struct {
	Format        string `yaml:"format"` // json, text, prometheus
	File          string `yaml:"file"`
	IncludeEvents bool   `yaml:"include_events"`
	IncludeStats  bool   `yaml:"include_stats"`
}

// AuditConfig defines validation against audit service
type AuditConfig struct {
	ValidateKafka       bool   `yaml:"validate_kafka"`
	KafkaBroker         string `yaml:"kafka_broker"`
	KafkaTopic          string `yaml:"kafka_topic"`
	KafkaTimeoutSeconds int    `yaml:"kafka_timeout_seconds"`
}

// LoadConfig loads configuration from YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Validate and set defaults
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate checks configuration for required fields and valid values
func (c *Config) Validate() error {
	if c.Server.URL == "" {
		return fmt.Errorf("server.url is required")
	}

	// Check for api_key in config or environment variable
	if c.Server.APIKey == "" {
		// Try to get from TINODE_API_KEY environment variable
		if apiKey := os.Getenv("TINODE_API_KEY"); apiKey != "" {
			c.Server.APIKey = apiKey
		} else {
			return fmt.Errorf("server.api_key is required (set in config or TINODE_API_KEY env var)")
		}
	}

	if len(c.Users) == 0 {
		return fmt.Errorf("at least one user must be defined")
	}

	// Validate each user has required fields
	for i, u := range c.Users {
		if u.ID == "" {
			return fmt.Errorf("user %d: id is required", i)
		}
		if u.Login == "" {
			return fmt.Errorf("user %d: login is required", i)
		}
		if u.Password == "" {
			return fmt.Errorf("user %d: password is required", i)
		}
	}

	// Set defaults
	if c.Generator.MaxConcurrency == 0 {
		c.Generator.MaxConcurrency = 10
	}
	if c.Generator.LogLevel == "" {
		c.Generator.LogLevel = "info"
	}
	if c.Generator.LogOutput == "" {
		c.Generator.LogOutput = "stdout"
	}
	if c.Server.TimeoutSecs == 0 {
		c.Server.TimeoutSecs = 30
	}

	return nil
}

// GetUser returns user config by ID
func (c *Config) GetUser(id string) *UserConfig {
	for i := range c.Users {
		if c.Users[i].ID == id {
			return &c.Users[i]
		}
	}
	return nil
}

// GetScenarioTimeout returns timeout for a scenario with default fallback
func (c *Config) GetScenarioTimeout(timeoutSecs int, defaultSecs int) time.Duration {
	if timeoutSecs > 0 {
		return time.Duration(timeoutSecs) * time.Second
	}
	return time.Duration(defaultSecs) * time.Second
}
