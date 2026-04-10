package scenario

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/echomessenger/generator/internal/client"
	"github.com/echomessenger/generator/internal/config"
	"github.com/sirupsen/logrus"
)

// NormalScenario simulates realistic p2p and group messaging
type NormalScenario struct {
	config       *config.NormalConfig
	serverConfig *config.ServerConfig
	users        map[string]config.UserConfig
	topics       map[string]config.TopicConfig
	runner       *Runner
	log          *logrus.Logger
}

// NewNormalScenario creates a new normal scenario
func NewNormalScenario(cfg *config.NormalConfig, serverCfg *config.ServerConfig, users map[string]config.UserConfig, topics map[string]config.TopicConfig, runner *Runner, log *logrus.Logger) *NormalScenario {
	return &NormalScenario{
		config:       cfg,
		serverConfig: serverCfg,
		users:        users,
		topics:       topics,
		runner:       runner,
		log:          log,
	}
}

// Name returns scenario name
func (ns *NormalScenario) Name() string {
	return "normal"
}

// Description returns scenario description
func (ns *NormalScenario) Description() string {
	return fmt.Sprintf("Baseline normal traffic: %d users, %.1f msg/min, %d seconds",
		len(ns.config.UserIDs), float64(ns.config.MessagesPerMinute), ns.config.DurationSecs)
}

// Run executes the normal scenario
func (ns *NormalScenario) Run(ctx context.Context) error {
	if len(ns.config.UserIDs) == 0 {
		return fmt.Errorf("no users configured for normal scenario")
	}

	// Create context with timeout
	scenarioCtx, cancel := context.WithTimeout(ctx, time.Duration(ns.config.DurationSecs)*time.Second)
	defer cancel()

	// Calculate message interval
	intervalMs := int(60 * 1000 / ns.config.MessagesPerMinute)

	// Run each user in parallel
	wg := sync.WaitGroup{}
	errCh := make(chan error, len(ns.config.UserIDs))

	for _, userID := range ns.config.UserIDs {
		wg.Add(1)
		go func(uid string) {
			defer wg.Done()
			if err := ns.runUser(scenarioCtx, uid, intervalMs); err != nil {
				ns.log.Warnf("[%s] User %s error: %v", ns.Name(), uid, err)
				errCh <- err
			}
		}(userID)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	for err := range errCh {
		if err != nil {
			return fmt.Errorf("normal scenario error: %w", err)
		}
	}

	return nil
}

// runUser simulates a single user's behavior
func (ns *NormalScenario) runUser(ctx context.Context, userID string, intervalMs int) error {
	// Get user config
	user, ok := ns.users[userID]
	if !ok {
		return fmt.Errorf("user not found: %s", userID)
	}

	// Connect and authenticate
	wsClient := client.NewClient(ns.serverConfig.URL, ns.serverConfig.APIKey, ns.log)
	if err := wsClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer wsClient.Close()

	var session *client.Session
	if ns.runner.keycloakClient != nil {
		session = client.NewSessionWithKeycloak(wsClient, user.Login, user.Password, ns.runner.keycloakClient, ns.log)
	} else {
		session = client.NewSession(wsClient, user.Login, user.Password, ns.log)
	}
	if err := session.Connect(ctx); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	defer session.Close()

	actions := client.NewActions(session, ns.log)

	// Get list of topics to publish to
	topics := ns.getTopicsForUser(userID)
	if len(topics) == 0 {
		ns.log.Warnf("[%s] No topics for user %s", ns.Name(), userID)
		return nil
	}

	// Publish messages at configured rate
	messageCount := 0
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			ns.log.Debugf("[%s] User %s completed: %d messages", ns.Name(), userID, messageCount)
			return nil
		case <-time.After(time.Duration(intervalMs) * time.Millisecond):
			// Time to send a message
			topic := topics[rand.Intn(len(topics))]
			content := fmt.Sprintf("Normal message from %s at %s", userID, time.Now().Format(time.RFC3339))

			if err := actions.PublishMessage(ctx, topic, content); err != nil {
				ns.log.Warnf("[%s] Failed to publish to %s: %v", ns.Name(), topic, err)
				ns.runner.RecordError(ns.Name())
				continue
			}

			messageCount++
			ns.runner.RecordMessage(ns.Name())

			elapsed := time.Since(startTime).Seconds()
			if int(elapsed) >= ns.config.DurationSecs {
				ns.log.Debugf("[%s] User %s timeout reached", ns.Name(), userID)
				return nil
			}
		}
	}
}

// getTopicsForUser returns topics a user should publish to
func (ns *NormalScenario) getTopicsForUser(userID string) []string {
	// Return configured topics or fallback to defaults
	var topics []string

	// First, try to get topics from config (if provided)
	for topicID := range ns.topics {
		topics = append(topics, topicID)
	}

	// If no topics configured, return defaults
	if len(topics) == 0 {
		topics = []string{
			fmt.Sprintf("p2p-%s", userID),
			"grp-general",
			"grp-random",
		}
	}

	return topics
}

// ConcurrentSessionsScenario simulates N parallel sessions from same user
type ConcurrentSessionsScenario struct {
	config       *config.ConcurrentSessionsConfig
	serverConfig *config.ServerConfig
	users        map[string]config.UserConfig
	runner       *Runner
	log          *logrus.Logger
}

// NewConcurrentSessionsScenario creates a new concurrent sessions scenario
func NewConcurrentSessionsScenario(cfg *config.ConcurrentSessionsConfig, serverCfg *config.ServerConfig, users map[string]config.UserConfig, runner *Runner, log *logrus.Logger) *ConcurrentSessionsScenario {
	return &ConcurrentSessionsScenario{
		config:       cfg,
		serverConfig: serverCfg,
		users:        users,
		runner:       runner,
		log:          log,
	}
}

// Name returns scenario name
func (cs *ConcurrentSessionsScenario) Name() string {
	return "concurrent_sessions"
}

// Description returns scenario description
func (cs *ConcurrentSessionsScenario) Description() string {
	return fmt.Sprintf("R2: %d parallel sessions from user %s", cs.config.SessionCount, cs.config.UserID)
}

// Run executes the concurrent sessions scenario
func (cs *ConcurrentSessionsScenario) Run(ctx context.Context) error {
	user, ok := cs.users[cs.config.UserID]
	if !ok {
		return fmt.Errorf("user not found: %s", cs.config.UserID)
	}

	wg := sync.WaitGroup{}
	errCh := make(chan error, cs.config.SessionCount)

	for i := 0; i < cs.config.SessionCount; i++ {
		wg.Add(1)
		go func(sessionNum int) {
			defer wg.Done()
			if err := cs.runSession(ctx, user, sessionNum); err != nil {
				cs.log.Warnf("[%s] Session %d error: %v", cs.Name(), sessionNum, err)
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	for err := range errCh {
		if err != nil {
			return fmt.Errorf("concurrent sessions error: %w", err)
		}
	}

	return nil
}

// runSession runs a single session
func (cs *ConcurrentSessionsScenario) runSession(ctx context.Context, user config.UserConfig, sessionNum int) error {
	wsClient := client.NewClient(cs.serverConfig.URL, cs.serverConfig.APIKey, cs.log)
	if err := wsClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer wsClient.Close()

	var session *client.Session
	if cs.runner.keycloakClient != nil {
		session = client.NewSessionWithKeycloak(wsClient, user.Login, user.Password, cs.runner.keycloakClient, cs.log)
	} else {
		session = client.NewSession(wsClient, user.Login, user.Password, cs.log)
	}
	if err := session.Connect(ctx); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	defer session.Close()

	cs.log.Debugf("[%s] Session %d authenticated, holding for %.1f seconds",
		cs.Name(), sessionNum, float64(cs.config.DurationSecs))

	// Hold connection open for configured duration
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(cs.config.DurationSecs) * time.Second):
		cs.runner.RecordMessage(cs.Name())
		return nil
	}
}
