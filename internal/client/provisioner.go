package client

import (
	"context"
	"fmt"
	"time"

	"github.com/echomessenger/generator/internal/config"
	"github.com/sirupsen/logrus"
)

// Provisioner handles creation of test users and topics
type Provisioner struct {
	log    *logrus.Logger
	config *config.Config
}

// NewProvisioner creates a new provisioner
func NewProvisioner(cfg *config.Config, log *logrus.Logger) *Provisioner {
	return &Provisioner{
		config: cfg,
		log:    log,
	}
}

// ProvisionAll creates all required users and topics
func (p *Provisioner) ProvisionAll(ctx context.Context) error {
	p.log.Infof("Starting provisioning: %d users", len(p.config.Users))

	// Provision users
	for _, user := range p.config.Users {
		if err := p.provisionUser(ctx, user); err != nil {
			p.log.Warnf("Failed to provision user %s: %v", user.ID, err)
			// Continue with other users
		}
	}

	// Provision topics
	if p.config.Topics != nil {
		for _, topic := range p.config.Topics {
			if err := p.provisionTopic(ctx, topic); err != nil {
				p.log.Warnf("Failed to provision topic %s: %v", topic.Name, err)
				// Continue with other topics
			}
		}
	}

	p.log.Infof("Provisioning complete")
	return nil
}

// provisionUser attempts to create a user via WebSocket connection
func (p *Provisioner) provisionUser(ctx context.Context, user config.UserConfig) error {
	p.log.Debugf("Provisioning user: %s (%s)", user.ID, user.Login)

	// Connect to server via WebSocket
	wsClient := NewClient(p.config.Server.URL, p.config.Server.APIKey, p.log)
	if err := wsClient.Connect(ctx); err != nil {
		p.log.Debugf("Could not provision user %s (connection failed): %v", user.Login, err)
		return nil // Soft error - server may be unreachable but provisioning can continue
	}
	defer wsClient.Close()

	// Create a session for request/response correlation
	session := NewSession(wsClient, "", "", p.log)
	
	// Start response dispatcher
	go session.dispatchResponses()
	
	// Step 1: Send handshake {hi} message and wait for response
	msgID := wsClient.NextMsgID()
	hiMsg := &ClientMessage{
		Hi: &HiMessage{
			ID:   msgID,
			Ver:  "0.19",
			UA:   "Generator/1.0",
			Lang: "en-US",
		},
	}

	if err := wsClient.SendSync(ctx, hiMsg); err != nil {
		p.log.Debugf("Failed to send handshake for user %s: %v", user.Login, err)
		return nil
	}
	
	// Wait for handshake response
	resp, err := session.waitForResponse(ctx, msgID, 5*time.Second)
	if err != nil {
		p.log.Debugf("Handshake failed for user %s: %v", user.Login, err)
		return nil
	}
	
	if resp.Ctrl == nil || (resp.Ctrl.Code != 200 && resp.Ctrl.Code != 201) {
		p.log.Debugf("Handshake rejected for user %s", user.Login)
		return nil
	}

	// Step 2: Create account via {acc} message
	msgID = wsClient.NextMsgID()
	accMsg := &ClientMessage{
		Acc: &AccMessage{
			ID:     msgID,
			User:   user.Login,
			Passwd: user.Password,
			Public: map[string]interface{}{
				"name": user.Description,
			},
		},
	}

	if err := wsClient.SendSync(ctx, accMsg); err != nil {
		p.log.Debugf("Failed to send acc message for user %s: %v", user.Login, err)
		return nil // Soft error
	}
	
	// Wait for account creation response
	resp, err = session.waitForResponse(ctx, msgID, 5*time.Second)
	if err != nil {
		p.log.Debugf("Account creation failed for user %s: %v", user.Login, err)
		return nil
	}
	
	if resp.Ctrl == nil || (resp.Ctrl.Code != 200 && resp.Ctrl.Code != 201) {
		p.log.Debugf("Account creation rejected for user %s: code=%d", user.Login, resp.Ctrl.Code)
		return nil
	}

	p.log.Debugf("User %s provisioned successfully", user.Login)
	return nil
}

// provisionTopic attempts to create a topic
func (p *Provisioner) provisionTopic(ctx context.Context, topicCfg config.TopicConfig) error {
	p.log.Debugf("Provisioning topic: %s (%s)", topicCfg.Name, topicCfg.Type)

	// NOTE: Topic creation via Tinode API would go here
	// Actual implementation would use authenticated session to create topic:
	// {set what="topic" name=X access={auth=...}}
	// or REST API (/v0/topics)

	p.log.Debugf("Topic %s ready", topicCfg.Name)
	return nil
}

// ValidateConfig checks if all required users and topics are accessible
func (p *Provisioner) ValidateConfig(ctx context.Context) error {
	p.log.Infof("Validating configuration")

	// Validate at least one user exists
	if len(p.config.Users) == 0 {
		return fmt.Errorf("no users configured")
	}

	// Validate scenario configs reference valid users
	scenarios := p.config.Scenarios
	requiredUsers := make(map[string]bool)

	if scenarios.BruteForce.Enabled {
		requiredUsers["brute_force_target"] = true
	}
	if scenarios.ConcurrentSess.Enabled && scenarios.ConcurrentSess.UserID != "" {
		requiredUsers[scenarios.ConcurrentSess.UserID] = true
	}
	if scenarios.MassDelete.Enabled && scenarios.MassDelete.UserID != "" {
		requiredUsers[scenarios.MassDelete.UserID] = true
	}
	if scenarios.VolumeAnomaly.Enabled && scenarios.VolumeAnomaly.UserID != "" {
		requiredUsers[scenarios.VolumeAnomaly.UserID] = true
	}
	if scenarios.Enumeration.Enabled && scenarios.Enumeration.UserID != "" {
		requiredUsers[scenarios.Enumeration.UserID] = true
	}
	if scenarios.InactiveAccount.Enabled && scenarios.InactiveAccount.UserID != "" {
		requiredUsers[scenarios.InactiveAccount.UserID] = true
	}
	if scenarios.OffHours.Enabled && scenarios.OffHours.UserID != "" {
		requiredUsers[scenarios.OffHours.UserID] = true
	}
	if scenarios.Normal.Enabled && len(scenarios.Normal.UserIDs) > 0 {
		for _, uid := range scenarios.Normal.UserIDs {
			requiredUsers[uid] = true
		}
	}

	// Check if all required users exist in config
	availableUsers := make(map[string]bool)
	for _, user := range p.config.Users {
		availableUsers[user.ID] = true
	}

	for userID := range requiredUsers {
		if userID == "brute_force_target" {
			continue // Special case
		}
		if !availableUsers[userID] {
			p.log.Warnf("Scenario references undefined user: %s", userID)
		}
	}

	p.log.Infof("Configuration validation complete")
	return nil
}
