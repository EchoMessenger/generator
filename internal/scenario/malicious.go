package scenario

import (
	"context"
	"fmt"
	"time"

	"github.com/echomessenger/generator/internal/client"
	"github.com/echomessenger/generator/internal/config"
	"github.com/sirupsen/logrus"
)

// BruteForceScenario simulates multiple failed login attempts (R1)
type BruteForceScenario struct {
	config       *config.BruteForceConfig
	serverConfig *config.ServerConfig
	users        map[string]config.UserConfig
	runner       *Runner
	log          *logrus.Logger
}

// NewBruteForceScenario creates a new brute force scenario
func NewBruteForceScenario(cfg *config.BruteForceConfig, serverCfg *config.ServerConfig, users map[string]config.UserConfig, runner *Runner, log *logrus.Logger) *BruteForceScenario {
	return &BruteForceScenario{
		config:       cfg,
		serverConfig: serverCfg,
		users:        users,
		runner:       runner,
		log:          log,
	}
}

// Name returns scenario name
func (bs *BruteForceScenario) Name() string {
	return "brute_force"
}

// Description returns scenario description
func (bs *BruteForceScenario) Description() string {
	attempts := bs.config.MinAttempts
	if attempts <= 0 {
		attempts = len(bs.config.Attempts)
	}
	return fmt.Sprintf("R1: %d failed login attempts against %s", attempts, bs.config.TargetLogin)
}

// Run executes the brute force scenario
func (bs *BruteForceScenario) Run(ctx context.Context) error {
	attempts := bs.config.MinAttempts
	if attempts <= 0 {
		attempts = len(bs.config.Attempts)
	}

	bs.log.Infof("[%s] Starting %d login attempts against %s", bs.Name(), attempts, bs.config.TargetLogin)

	successCount := 0
	failureCount := 0

	for i := 0; i < attempts; i++ {
		select {
		case <-ctx.Done():
			bs.log.Warnf("[%s] Cancelled at attempt %d/%d", bs.Name(), i, attempts)
			return ctx.Err()
		default:
		}

		// Create new connection for each attempt
		wsClient := client.NewClient(bs.serverConfig.URL, bs.serverConfig.APIKey, bs.log)
		if err := wsClient.Connect(ctx); err != nil {
			bs.log.Debugf("[%s] Attempt %d: connection error: %v", bs.Name(), i+1, err)
			failureCount++
			continue
		}

		// Get password (cyclic from config or use wrong password)
		var password string
		if len(bs.config.Attempts) > 0 {
			password = bs.config.Attempts[i%len(bs.config.Attempts)]
		} else {
			password = fmt.Sprintf("wrong-password-%d", i)
		}

		session := client.NewSession(wsClient, bs.config.TargetLogin, password, bs.log)

		if err := session.Connect(ctx); err != nil {
			// Expected: login should fail with wrong password
			failureCount++
			bs.runner.RecordError(bs.Name())
		} else {
			// Unexpected: login succeeded? This shouldn't happen with wrong password
			bs.log.Warnf("[%s] Attempt %d: unexpected success!", bs.Name(), i+1)
			successCount++
			session.Close()
		}

		wsClient.Close()

		// Wait between attempts
		if i < attempts-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(bs.config.IntervalMs) * time.Millisecond):
			}
		}
	}

	bs.log.Infof("[%s] Completed: %d failures, %d unexpected successes", bs.Name(), failureCount, successCount)
	bs.runner.RecordMessage(bs.Name())
	return nil
}

// MassDeleteScenario simulates rapid message deletion (R3)
type MassDeleteScenario struct {
	config       *config.MassDeleteConfig
	serverConfig *config.ServerConfig
	users        map[string]config.UserConfig
	runner       *Runner
	log          *logrus.Logger
}

// NewMassDeleteScenario creates a new mass delete scenario
func NewMassDeleteScenario(cfg *config.MassDeleteConfig, serverCfg *config.ServerConfig, users map[string]config.UserConfig, runner *Runner, log *logrus.Logger) *MassDeleteScenario {
	return &MassDeleteScenario{
		config:       cfg,
		serverConfig: serverCfg,
		users:        users,
		runner:       runner,
		log:          log,
	}
}

// Name returns scenario name
func (mds *MassDeleteScenario) Name() string {
	return "mass_delete"
}

// Description returns scenario description
func (mds *MassDeleteScenario) Description() string {
	return fmt.Sprintf("R3: %d rapid deletions from user %s to peer %s", mds.config.DeleteBurstCount, mds.config.UserID, mds.config.PeerUserID)
}

// Run executes the mass delete scenario
func (mds *MassDeleteScenario) Run(ctx context.Context) error {
	user, ok := mds.users[mds.config.UserID]
	if !ok {
		return fmt.Errorf("user not found: %s", mds.config.UserID)
	}

	scenarioCtx, cancel := context.WithTimeout(ctx, time.Duration(mds.config.TimeoutSecs)*time.Second)
	defer cancel()

	wsClient := client.NewClient(mds.serverConfig.URL, mds.serverConfig.APIKey, mds.log)
	if err := wsClient.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer wsClient.Close()

	session := client.NewSession(wsClient, user.Login, user.Password, mds.log)
	if err := session.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	defer session.Close()

	// Create topic name for p2p conversation
	topicID := fmt.Sprintf("p2p-%s-%s", user.Login, mds.config.PeerUserID)

	// Subscribe to topic
	if err := session.Subscribe(scenarioCtx, topicID); err != nil {
		mds.log.Debugf("[%s] Failed to subscribe to %s: %v", mds.Name(), topicID, err)
	}

	actions := client.NewActions(session, mds.log)

	// Delete messages in rapid succession
	deleteCount := 0
	for i := 0; i < mds.config.DeleteBurstCount; i++ {
		select {
		case <-scenarioCtx.Done():
			return scenarioCtx.Err()
		default:
		}

		// Delete a range of messages
		if err := actions.DeleteMessageRange(scenarioCtx, topicID, i, i+1); err != nil {
			mds.log.Debugf("[%s] Delete %d failed: %v", mds.Name(), i, err)
			mds.runner.RecordError(mds.Name())
		} else {
			deleteCount++
			mds.runner.RecordMessage(mds.Name())
		}

		// Wait between deletes
		if i < mds.config.DeleteBurstCount-1 {
			select {
			case <-scenarioCtx.Done():
				return scenarioCtx.Err()
			case <-time.After(time.Duration(mds.config.DeleteIntervalMs) * time.Millisecond):
			}
		}
	}

	mds.log.Infof("[%s] Completed: %d messages deleted", mds.Name(), deleteCount)
	return nil
}

// VolumeAnomalyScenario simulates high-frequency publishing (R4)
type VolumeAnomalyScenario struct {
	config       *config.VolumeAnomalyConfig
	serverConfig *config.ServerConfig
	users        map[string]config.UserConfig
	runner       *Runner
	log          *logrus.Logger
}

// NewVolumeAnomalyScenario creates a new volume anomaly scenario
func NewVolumeAnomalyScenario(cfg *config.VolumeAnomalyConfig, serverCfg *config.ServerConfig, users map[string]config.UserConfig, runner *Runner, log *logrus.Logger) *VolumeAnomalyScenario {
	return &VolumeAnomalyScenario{
		config:       cfg,
		serverConfig: serverCfg,
		users:        users,
		runner:       runner,
		log:          log,
	}
}

// Name returns scenario name
func (vas *VolumeAnomalyScenario) Name() string {
	return "volume_anomaly"
}

// Description returns scenario description
func (vas *VolumeAnomalyScenario) Description() string {
	return fmt.Sprintf("R4: %d messages/min from user %s in %s for %d seconds", vas.config.MessagesPerMinute, vas.config.UserID, vas.config.TopicName, vas.config.DurationSecs)
}

// Run executes the volume anomaly scenario
func (vas *VolumeAnomalyScenario) Run(ctx context.Context) error {
	user, ok := vas.users[vas.config.UserID]
	if !ok {
		return fmt.Errorf("user not found: %s", vas.config.UserID)
	}

	scenarioCtx, cancel := context.WithTimeout(ctx, time.Duration(vas.config.DurationSecs)*time.Second)
	defer cancel()

	wsClient := client.NewClient(vas.serverConfig.URL, vas.serverConfig.APIKey, vas.log)
	if err := wsClient.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer wsClient.Close()

	session := client.NewSession(wsClient, user.Login, user.Password, vas.log)
	if err := session.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	defer session.Close()

	// Use configured topic or default
	topic := vas.config.TopicName
	if topic == "" {
		topic = fmt.Sprintf("p2p-%s", user.Login)
	}

	if err := session.Subscribe(scenarioCtx, topic); err != nil {
		vas.log.Debugf("[%s] Failed to subscribe: %v (continuing)", vas.Name(), err)
	}

	actions := client.NewActions(session, vas.log)

	// Publish at high frequency
	messageCount := 0
	intervalMs := int(60 * 1000 / vas.config.MessagesPerMinute)

	for {
		select {
		case <-scenarioCtx.Done():
			vas.log.Infof("[%s] Completed: %d messages sent", vas.Name(), messageCount)
			return nil
		case <-time.After(time.Duration(intervalMs) * time.Millisecond):
			content := vas.config.MessageContent
			if content == "" {
				content = fmt.Sprintf("High-volume message %d from %s", messageCount, user.Login)
			}

			if err := actions.PublishMessage(scenarioCtx, topic, content); err != nil {
				vas.log.Debugf("[%s] Publish failed: %v", vas.Name(), err)
				vas.runner.RecordError(vas.Name())
			} else {
				messageCount++
				vas.runner.RecordMessage(vas.Name())
			}
		}
	}
}

// EnumerationScenario simulates subscription attempts to closed topics (R5)
type EnumerationScenario struct {
	config       *config.EnumerationConfig
	serverConfig *config.ServerConfig
	users        map[string]config.UserConfig
	runner       *Runner
	log          *logrus.Logger
}

// NewEnumerationScenario creates a new enumeration scenario
func NewEnumerationScenario(cfg *config.EnumerationConfig, serverCfg *config.ServerConfig, users map[string]config.UserConfig, runner *Runner, log *logrus.Logger) *EnumerationScenario {
	return &EnumerationScenario{
		config:       cfg,
		serverConfig: serverCfg,
		users:        users,
		runner:       runner,
		log:          log,
	}
}

// Name returns scenario name
func (es *EnumerationScenario) Name() string {
	return "enumeration"
}

// Description returns scenario description
func (es *EnumerationScenario) Description() string {
	return fmt.Sprintf("R5: %d topic enumeration attempts from user %s", len(es.config.TopicIDs), es.config.UserID)
}

// Run executes the enumeration scenario
func (es *EnumerationScenario) Run(ctx context.Context) error {
	user, ok := es.users[es.config.UserID]
	if !ok {
		return fmt.Errorf("user not found: %s", es.config.UserID)
	}

	scenarioCtx, cancel := context.WithTimeout(ctx, time.Duration(es.config.TimeoutSecs)*time.Second)
	defer cancel()

	wsClient := client.NewClient(es.serverConfig.URL, es.serverConfig.APIKey, es.log)
	if err := wsClient.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer wsClient.Close()

	session := client.NewSession(wsClient, user.Login, user.Password, es.log)
	if err := session.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	defer session.Close()

	// Attempt to subscribe to each topic
	successCount := 0
	deniedCount := 0

	for _, topicID := range es.config.TopicIDs {
		select {
		case <-scenarioCtx.Done():
			return scenarioCtx.Err()
		default:
		}

		if err := session.Subscribe(scenarioCtx, topicID); err != nil {
			// Expected: should fail with permission denied for closed topics
			deniedCount++
			es.runner.RecordError(es.Name())
			es.log.Debugf("[%s] Topic %s: access denied (expected)", es.Name(), topicID)
		} else {
			// Unexpected: should not have access
			successCount++
			es.runner.RecordMessage(es.Name())
			es.log.Warnf("[%s] Topic %s: unexpected access granted", es.Name(), topicID)
			session.Leave(scenarioCtx, topicID, false)
		}

		// Wait between attempts
		if es.config.IntervalMs > 0 {
			select {
			case <-scenarioCtx.Done():
				return scenarioCtx.Err()
			case <-time.After(time.Duration(es.config.IntervalMs) * time.Millisecond):
			}
		}
	}

	es.log.Infof("[%s] Completed: %d denied, %d unexpected granted", es.Name(), deniedCount, successCount)
	return nil
}

// InactiveAccountScenario simulates sudden activity from inactive account (R6)
type InactiveAccountScenario struct {
	config       *config.InactiveAccountConfig
	serverConfig *config.ServerConfig
	users        map[string]config.UserConfig
	runner       *Runner
	log          *logrus.Logger
}

// NewInactiveAccountScenario creates a new inactive account scenario
func NewInactiveAccountScenario(cfg *config.InactiveAccountConfig, serverCfg *config.ServerConfig, users map[string]config.UserConfig, runner *Runner, log *logrus.Logger) *InactiveAccountScenario {
	return &InactiveAccountScenario{
		config:       cfg,
		serverConfig: serverCfg,
		users:        users,
		runner:       runner,
		log:          log,
	}
}

// Name returns scenario name
func (ias *InactiveAccountScenario) Name() string {
	return "inactive_account"
}

// Description returns scenario description
func (ias *InactiveAccountScenario) Description() string {
	return fmt.Sprintf("R6: Sudden activity from user %s (%d messages)", ias.config.UserID, ias.config.MessageBurstCount)
}

// Run executes the inactive account scenario
func (ias *InactiveAccountScenario) Run(ctx context.Context) error {
	user, ok := ias.users[ias.config.UserID]
	if !ok {
		return fmt.Errorf("user not found: %s", ias.config.UserID)
	}

	ias.log.Warnf("[%s] NOTE: Ensure user %s has been inactive for 30+ days before running this scenario", ias.Name(), ias.config.UserID)

	scenarioCtx, cancel := context.WithTimeout(ctx, time.Duration(ias.config.TimeoutSecs)*time.Second)
	defer cancel()

	wsClient := client.NewClient(ias.serverConfig.URL, ias.serverConfig.APIKey, ias.log)
	if err := wsClient.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer wsClient.Close()

	session := client.NewSession(wsClient, user.Login, user.Password, ias.log)
	if err := session.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	defer session.Close()

	actions := client.NewActions(session, ias.log)

	// Use configured topic or default
	topic := ias.config.TopicName
	if topic == "" {
		topic = fmt.Sprintf("p2p-%s", user.Login)
	}

	if err := session.Subscribe(scenarioCtx, topic); err != nil {
		ias.log.Debugf("[%s] Failed to subscribe: %v", ias.Name(), err)
	}

	// Send burst of messages
	for i := 0; i < ias.config.MessageBurstCount; i++ {
		select {
		case <-scenarioCtx.Done():
			return scenarioCtx.Err()
		default:
		}

		content := fmt.Sprintf("Sudden activity from inactive account [%d/%d]", i+1, ias.config.MessageBurstCount)
		if err := actions.PublishMessage(scenarioCtx, topic, content); err != nil {
			ias.log.Debugf("[%s] Publish %d failed: %v", ias.Name(), i+1, err)
			ias.runner.RecordError(ias.Name())
		} else {
			ias.runner.RecordMessage(ias.Name())
		}

		// Wait between messages
		if ias.config.IntervalMs > 0 {
			select {
			case <-scenarioCtx.Done():
				return scenarioCtx.Err()
			case <-time.After(time.Duration(ias.config.IntervalMs) * time.Millisecond):
			}
		}
	}

	ias.log.Infof("[%s] Completed: %d messages sent from inactive account", ias.Name(), ias.config.MessageBurstCount)
	return nil
}

// OffHoursScenario simulates activity outside business hours (R7)
type OffHoursScenario struct {
	config       *config.OffHoursConfig
	serverConfig *config.ServerConfig
	users        map[string]config.UserConfig
	runner       *Runner
	log          *logrus.Logger
}

// NewOffHoursScenario creates a new off-hours scenario
func NewOffHoursScenario(cfg *config.OffHoursConfig, serverCfg *config.ServerConfig, users map[string]config.UserConfig, runner *Runner, log *logrus.Logger) *OffHoursScenario {
	return &OffHoursScenario{
		config:       cfg,
		serverConfig: serverCfg,
		users:        users,
		runner:       runner,
		log:          log,
	}
}

// Name returns scenario name
func (ohs *OffHoursScenario) Name() string {
	return "off_hours"
}

// Description returns scenario description
func (ohs *OffHoursScenario) Description() string {
	return fmt.Sprintf("R7: %d messages/min from user %s for %d seconds (off-hours)", ohs.config.MessagesPerMinute, ohs.config.UserID, ohs.config.DurationSecs)
}

// Run executes the off-hours scenario
func (ohs *OffHoursScenario) Run(ctx context.Context) error {
	user, ok := ohs.users[ohs.config.UserID]
	if !ok {
		return fmt.Errorf("user not found: %s", ohs.config.UserID)
	}

	// Check if within configured business hours window
	if ohs.config.BusinessHoursStart != "" && ohs.config.BusinessHoursEnd != "" {
		now := time.Now()
		startTime, _ := time.Parse("15:04", ohs.config.BusinessHoursStart)
		endTime, _ := time.Parse("15:04", ohs.config.BusinessHoursEnd)

		nowTime := time.Date(0, 0, 0, now.Hour(), now.Minute(), 0, 0, time.UTC)

		if startTime.Before(endTime) {
			// Normal range (e.g., 09:00-18:00)
			if nowTime.After(startTime) && nowTime.Before(endTime) {
				ohs.log.Warnf("[%s] Within business hours (%s-%s), proceeding anyway",
					ohs.Name(), ohs.config.BusinessHoursStart, ohs.config.BusinessHoursEnd)
			}
		} else {
			// Wrapped range (e.g., 18:00-09:00)
			if nowTime.After(startTime) || nowTime.Before(endTime) {
				ohs.log.Warnf("[%s] Within business hours (%s-%s), proceeding anyway",
					ohs.Name(), ohs.config.BusinessHoursStart, ohs.config.BusinessHoursEnd)
			}
		}
	}

	scenarioCtx, cancel := context.WithTimeout(ctx, time.Duration(ohs.config.DurationSecs)*time.Second)
	defer cancel()

	wsClient := client.NewClient(ohs.serverConfig.URL, ohs.serverConfig.APIKey, ohs.log)
	if err := wsClient.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer wsClient.Close()

	session := client.NewSession(wsClient, user.Login, user.Password, ohs.log)
	if err := session.Connect(scenarioCtx); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	defer session.Close()

	// Use configured topic or default
	topic := ohs.config.TopicName
	if topic == "" {
		topic = fmt.Sprintf("p2p-%s", user.Login)
	}

	if err := session.Subscribe(scenarioCtx, topic); err != nil {
		ohs.log.Debugf("[%s] Failed to subscribe: %v", ohs.Name(), err)
	}

	actions := client.NewActions(session, ohs.log)

	// Publish at configured rate
	messageCount := 0
	intervalMs := int(60 * 1000 / ohs.config.MessagesPerMinute)

	for {
		select {
		case <-scenarioCtx.Done():
			ohs.log.Infof("[%s] Completed: %d messages sent during off-hours", ohs.Name(), messageCount)
			return nil
		case <-time.After(time.Duration(intervalMs) * time.Millisecond):
			content := fmt.Sprintf("Off-hours activity message [%d]", messageCount)

			if err := actions.PublishMessage(scenarioCtx, topic, content); err != nil {
				ohs.log.Debugf("[%s] Publish failed: %v", ohs.Name(), err)
				ohs.runner.RecordError(ohs.Name())
			} else {
				messageCount++
				ohs.runner.RecordMessage(ohs.Name())
			}
		}
	}
}
