package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/echomessenger/generator/internal/client"
	"github.com/echomessenger/generator/internal/config"
	"github.com/echomessenger/generator/internal/scenario"
	"github.com/sirupsen/logrus"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	var (
		configPath    = flag.String("config", "config.yaml", "Path to configuration file")
		dryRun        = flag.Bool("dry-run", false, "Dry run mode (log events, don't send)")
		logLevel      = flag.String("log-level", "info", "Log level: debug, info, warn, error")
		logFile       = flag.String("log-file", "", "Log file path (optional)")
		eventLog      = flag.String("events-log", "", "Events log file path (JSON format)")
		metricsAddr   = flag.String("metrics", ":8080", "Prometheus metrics address (e.g., :8080)")
		scenarios     = flag.String("scenarios", "", "Comma-separated list of scenarios to run (or 'all')")
		showVer       = flag.Bool("version", false, "Show version and exit")
		showHelp      = flag.Bool("help", false, "Show help and exit")
		listScenarios = flag.Bool("list-scenarios", false, "List available scenarios and exit")
	)

	flag.Parse()

	if *showVer {
		fmt.Printf("generator version %s (commit: %s)\n", version, commit)
		os.Exit(0)
	}

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	if *listScenarios {
		printScenarios()
		os.Exit(0)
	}

	// Setup logging
	log := setupLogging(*logLevel, *logFile)

	log.Infof("Generator %s starting (commit: %s)", version, commit)

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config from %s: %v", *configPath, err)
	}

	// Apply command-line overrides
	if *dryRun {
		cfg.Generator.DryRun = true
		log.Warn("Dry-run mode enabled: events will be logged but not sent to server")
	}

	// Store CLI overrides for later use
	cfg.Generator.LogFile = *logFile
	cfg.Generator.EventsLogFile = *eventLog
	cfg.Generator.MetricsAddr = *metricsAddr
	cfg.Generator.SelectedScenarios = *scenarios

	log.Debugf("Configuration loaded: %+v", cfg)

	// TODO: Initialize generator engine
	// - Create clients for each user
	// - Provision users and topics
	// - Run scenarios sequentially or in parallel
	// - Collect metrics and output results

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Infof("Received signal: %v", sig)
		cancel()
	}()

	// Run generator
	if err := run(ctx, cfg, log); err != nil {
		log.Fatalf("Generator failed: %v", err)
	}

	log.Info("Generator completed successfully")
}

func run(ctx context.Context, cfg *config.Config, log *logrus.Logger) error {
	// Validate configuration
	provisioner := client.NewProvisioner(cfg, log)
	if err := provisioner.ValidateConfig(ctx); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Provision users and topics if not in dry-run mode
	if !cfg.Generator.DryRun {
		log.Info("Provisioning users and verifying access")
		if err := provisioner.ProvisionAll(ctx); err != nil {
			log.Warnf("Provisioning warning: %v", err)
			// Don't fail on provisioning errors - users may already exist
		}
	} else {
		log.Info("Dry-run mode: skipping provisioning")
	}

	// Create runner with concurrency limits
	maxConcurrency := cfg.Generator.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}

	rateLimitPerSec := float64(cfg.Generator.RateLimitPerSecond)
	runner := scenario.NewRunner(log, maxConcurrency, rateLimitPerSec)

	log.Infof("Initialized runner: max concurrency=%d, rate limit=%.1f/sec", maxConcurrency, rateLimitPerSec)

	// Convert config users to map
	usersMap := make(map[string]config.UserConfig)
	for _, user := range cfg.Users {
		usersMap[user.ID] = user
	}

	// Convert config topics to map
	topicsMap := make(map[string]config.TopicConfig)
	if cfg.Topics != nil {
		for _, topic := range cfg.Topics {
			topicsMap[topic.Name] = topic
		}
	}

	// Register scenarios
	scenarioCount := 0

	// Normal baseline scenario
	if cfg.Scenarios.Normal.Enabled {
		s := scenario.NewNormalScenario(&cfg.Scenarios.Normal, &cfg.Server, usersMap, topicsMap, runner, log)
		runner.Register("normal", s)
		scenarioCount++
	}

	// Concurrent sessions (R2)
	if cfg.Scenarios.ConcurrentSess.Enabled {
		s := scenario.NewConcurrentSessionsScenario(&cfg.Scenarios.ConcurrentSess, &cfg.Server, usersMap, runner, log)
		runner.Register("concurrent_sessions", s)
		scenarioCount++
	}

	// Brute force (R1)
	if cfg.Scenarios.BruteForce.Enabled {
		s := scenario.NewBruteForceScenario(&cfg.Scenarios.BruteForce, &cfg.Server, usersMap, runner, log)
		runner.Register("brute_force", s)
		scenarioCount++
	}

	// Mass delete (R3)
	if cfg.Scenarios.MassDelete.Enabled {
		s := scenario.NewMassDeleteScenario(&cfg.Scenarios.MassDelete, &cfg.Server, usersMap, runner, log)
		runner.Register("mass_delete", s)
		scenarioCount++
	}

	// Volume anomaly (R4)
	if cfg.Scenarios.VolumeAnomaly.Enabled {
		s := scenario.NewVolumeAnomalyScenario(&cfg.Scenarios.VolumeAnomaly, &cfg.Server, usersMap, runner, log)
		runner.Register("volume_anomaly", s)
		scenarioCount++
	}

	// Enumeration (R5)
	if cfg.Scenarios.Enumeration.Enabled {
		s := scenario.NewEnumerationScenario(&cfg.Scenarios.Enumeration, &cfg.Server, usersMap, runner, log)
		runner.Register("enumeration", s)
		scenarioCount++
	}

	// Inactive account (R6)
	if cfg.Scenarios.InactiveAccount.Enabled {
		s := scenario.NewInactiveAccountScenario(&cfg.Scenarios.InactiveAccount, &cfg.Server, usersMap, runner, log)
		runner.Register("inactive_account", s)
		scenarioCount++
	}

	// Off-hours (R7)
	if cfg.Scenarios.OffHours.Enabled {
		s := scenario.NewOffHoursScenario(&cfg.Scenarios.OffHours, &cfg.Server, usersMap, runner, log)
		runner.Register("off_hours", s)
		scenarioCount++
	}

	if scenarioCount == 0 {
		return fmt.Errorf("no scenarios enabled in configuration")
	}

	log.Infof("Registered %d scenarios", scenarioCount)

	// Run scenarios
	log.Info("Starting scenario execution")
	if err := runner.Run(ctx); err != nil {
		return fmt.Errorf("scenario execution failed: %w", err)
	}

	// Print statistics
	runner.PrintStats()

	return nil
}

func setupLogging(level string, logFile string) *logrus.Logger {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// Set output to file if specified
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logrus.Warnf("Failed to open log file %s: %v, using stdout", logFile, err)
			log.SetOutput(os.Stdout)
		} else {
			log.SetOutput(f)
		}
	} else {
		log.SetOutput(os.Stdout)
	}

	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	log.SetLevel(logLevel)

	return log
}

func printHelp() {
	fmt.Println(`
EchoMessenger Incident Detection Test Generator

Usage:
  generator [flags]

Flags:
  -config string
        Path to configuration file (default "config.yaml")
  -dry-run
        Dry run mode (log events, don't send to server)
  -log-level string
        Log level: debug, info, warn, error (default "info")
  -log-file string
        Log file path (optional, defaults to stdout)
  -events-log string
        Events log file path (JSON format for audit trail)
  -metrics string
        Prometheus metrics address (default ":8080")
  -scenarios string
        Comma-separated list of scenarios to run, or "all" for all enabled scenarios
  -list-scenarios
        List available scenarios and exit
  -version
        Show version and exit
  -help
        Show this help message and exit

Examples:
  # Run with default config
  generator

  # Run with custom config
  generator -config ./my-config.yaml

  # Run specific scenarios
  generator -scenarios brute_force,volume_anomaly

  # Dry run with debug logging to file
  generator -dry-run -log-level debug -log-file generator.log

  # With events audit trail and metrics
  generator -events-log events.jsonl -metrics :8080

  # List available scenarios
  generator -list-scenarios

  # Show version
  generator -version

For more information, visit: https://github.com/echomessenger/generator
`)
}

func printScenarios() {
	fmt.Println(`
Available Scenarios:

Detection Rules (Malicious Behavior):
  normal              - Baseline realistic p2p + group messaging
  concurrent_sessions - R2: N parallel sessions from same user (potential account compromise)
  brute_force         - R1: Multiple failed login attempts (brute force attack)
  mass_delete         - R3: Rapid message deletion in conversation (data destruction)
  volume_anomaly      - R4: High-frequency publishing burst (spam/flooding)
  enumeration         - R5: Subscription attempts to restricted topics (reconnaissance)
  inactive_account    - R6: Sudden activity from dormant account (account takeover)
  off_hours           - R7: Normal traffic outside business hours (suspicious timing)

Usage:
  # Run all enabled scenarios from config
  generator

  # Run only specific scenarios
  generator -scenarios brute_force,volume_anomaly

  # Run all available scenarios
  generator -scenarios all

  # See full help
  generator -help
`)
}
