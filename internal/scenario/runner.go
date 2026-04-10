package scenario

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/echomessenger/generator/internal/client"
	"github.com/sirupsen/logrus"
)

// Scenario defines the interface for all test scenarios
type Scenario interface {
	// Name returns the scenario name
	Name() string

	// Description returns a human-readable description
	Description() string

	// Run executes the scenario with the given context
	// Should respect context cancellation
	Run(ctx context.Context) error
}

// Runner executes scenarios with concurrency control and rate limiting
type Runner struct {
	log              *logrus.Logger
	maxConcurrency   int
	rateLimitPerSec  float64
	scenarios        map[string]Scenario
	statsLock        sync.RWMutex
	stats            map[string]*ScenarioStats
	globalMessagesCh chan struct{} // Token bucket for rate limiting
	keycloakClient   *client.KeycloakClient
}

// ScenarioStats tracks execution metrics for a scenario
type ScenarioStats struct {
	Name          string
	StartTime     time.Time
	EndTime       time.Time
	MessagesCount int64
	ErrorsCount   int64
	Duration      time.Duration
	Success       bool
	LastError     string
}

// NewRunner creates a new scenario runner
func NewRunner(log *logrus.Logger, maxConcurrency int, rateLimitPerSec float64) *Runner {
	return &Runner{
		log:              log,
		maxConcurrency:   maxConcurrency,
		rateLimitPerSec:  rateLimitPerSec,
		scenarios:        make(map[string]Scenario),
		stats:            make(map[string]*ScenarioStats),
		globalMessagesCh: make(chan struct{}, 100), // Global rate limit buffer
	}
}

// SetKeycloakClient sets the Keycloak client for JWT auth
func (r *Runner) SetKeycloakClient(kc *client.KeycloakClient) {
	r.keycloakClient = kc
}

// Register adds a scenario to the runner
func (r *Runner) Register(name string, scenario Scenario) {
	r.scenarios[name] = scenario
	r.log.Debugf("Registered scenario: %s", name)
}

// Run executes all registered scenarios with concurrency control
func (r *Runner) Run(ctx context.Context) error {
	if len(r.scenarios) == 0 {
		return fmt.Errorf("no scenarios registered")
	}

	r.log.Infof("Starting runner with %d scenarios, max concurrency: %d", len(r.scenarios), r.maxConcurrency)

	// Start rate limiter
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go r.rateLimiter(ctx)

	// Run scenarios with limited concurrency
	semaphore := make(chan struct{}, r.maxConcurrency)
	wg := sync.WaitGroup{}
	errCh := make(chan error, len(r.scenarios))

	for name, scenario := range r.scenarios {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(scenarioName string, scenarioImpl Scenario) {
			defer func() {
				<-semaphore // Release semaphore
				wg.Done()
			}()

			if err := r.runScenario(ctx, scenarioName, scenarioImpl); err != nil {
				r.log.Errorf("Scenario %s failed: %v", scenarioName, err)
				errCh <- err
			}
		}(name, scenario)
	}

	// Wait for all scenarios to complete
	wg.Wait()
	close(errCh)

	// Collect errors
	var errors []error
	for err := range errCh {
		if err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d scenarios failed: %v", len(errors), errors[0])
	}

	r.PrintStats()
	return nil
}

// RunScenario runs a single scenario by name
func (r *Runner) RunScenario(ctx context.Context, name string) error {
	scenario, ok := r.scenarios[name]
	if !ok {
		return fmt.Errorf("scenario not found: %s", name)
	}

	return r.runScenario(ctx, name, scenario)
}

// runScenario is the internal implementation
func (r *Runner) runScenario(ctx context.Context, name string, scenario Scenario) error {
	stats := &ScenarioStats{
		Name:      name,
		StartTime: time.Now(),
	}
	defer func() {
		stats.EndTime = time.Now()
		stats.Duration = stats.EndTime.Sub(stats.StartTime)
		r.statsLock.Lock()
		r.stats[name] = stats
		r.statsLock.Unlock()
	}()

	r.log.Infof("[%s] Starting scenario: %s", name, scenario.Description())

	if err := scenario.Run(ctx); err != nil {
		stats.Success = false
		stats.LastError = err.Error()
		atomic.AddInt64(&stats.ErrorsCount, 1)
		return fmt.Errorf("scenario %s error: %w", name, err)
	}

	stats.Success = true
	r.log.Infof("[%s] Completed successfully in %v", name, stats.Duration)
	return nil
}

// rateLimiter implements token bucket rate limiting
func (r *Runner) rateLimiter(ctx context.Context) {
	if r.rateLimitPerSec <= 0 {
		// Unlimited
		for {
			select {
			case <-ctx.Done():
				return
			}
		}
	}

	ticker := time.NewTicker(time.Duration(float64(time.Second) / r.rateLimitPerSec))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case r.globalMessagesCh <- struct{}{}:
				// Token added to bucket
			default:
				// Bucket full, skip
			}
		}
	}
}

// AcquireRateLimit waits for a rate limit token
func (r *Runner) AcquireRateLimit(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.globalMessagesCh:
		return nil
	}
}

// RecordMessage increments message counter for a scenario
func (r *Runner) RecordMessage(name string) {
	r.statsLock.Lock()
	defer r.statsLock.Unlock()
	if stats, ok := r.stats[name]; ok {
		atomic.AddInt64(&stats.MessagesCount, 1)
	}
}

// RecordError increments error counter for a scenario
func (r *Runner) RecordError(name string) {
	r.statsLock.Lock()
	defer r.statsLock.Unlock()
	if stats, ok := r.stats[name]; ok {
		atomic.AddInt64(&stats.ErrorsCount, 1)
	}
}

// GetStats returns statistics for a scenario
func (r *Runner) GetStats(name string) *ScenarioStats {
	r.statsLock.RLock()
	defer r.statsLock.RUnlock()
	if stats, ok := r.stats[name]; ok {
		return stats
	}
	return nil
}

// PrintStats prints execution statistics for all scenarios
func (r *Runner) PrintStats() {
	r.statsLock.RLock()
	defer r.statsLock.RUnlock()

	if len(r.stats) == 0 {
		return
	}

	r.log.Info("================================================================================")
	r.log.Info("SCENARIO EXECUTION STATISTICS")
	r.log.Info("================================================================================")

	for name, stats := range r.stats {
		status := "✓ SUCCESS"
		if !stats.Success {
			status = "✗ FAILED"
		}

		r.log.Infof("[%s] %s", name, status)
		r.log.Infof("  Duration: %v", stats.Duration)
		r.log.Infof("  Messages: %d", stats.MessagesCount)
		r.log.Infof("  Errors: %d", stats.ErrorsCount)
		if stats.LastError != "" {
			r.log.Infof("  Last Error: %s", stats.LastError)
		}
	}

	r.log.Info("================================================================================")
}

// ScenarioBuilder is a helper to build scenarios with common patterns
type ScenarioBuilder struct {
	name        string
	description string
	runner      *Runner
	log         *logrus.Logger
}

// NewScenarioBuilder creates a new scenario builder
func NewScenarioBuilder(runner *Runner, name, description string, log *logrus.Logger) *ScenarioBuilder {
	return &ScenarioBuilder{
		name:        name,
		description: description,
		runner:      runner,
		log:         log,
	}
}

// Name returns the scenario name
func (sb *ScenarioBuilder) Name() string {
	return sb.name
}

// Description returns the scenario description
func (sb *ScenarioBuilder) Description() string {
	return sb.description
}

// Logger returns the logger
func (sb *ScenarioBuilder) Logger() *logrus.Logger {
	return sb.log
}

// Runner returns the runner
func (sb *ScenarioBuilder) Runner() *Runner {
	return sb.runner
}
