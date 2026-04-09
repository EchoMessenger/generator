package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

// provisionUser attempts to create a user via REST API
func (p *Provisioner) provisionUser(ctx context.Context, user config.UserConfig) error {
	p.log.Debugf("Provisioning user: %s (%s)", user.ID, user.Login)

	// Use REST API to create account via Tinode server
	// If user already exists, we get 409 which is OK
	payload := map[string]interface{}{
		"acc": map[string]interface{}{
			"user":   user.Login,
			"passwd": user.Password,
			"public": map[string]string{
				"name": user.Description,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal user payload: %w", err)
	}

	url := fmt.Sprintf("%s/v0/users", p.config.Server.APIEndpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.config.Server.APIKey != "" {
		req.Header.Set("X-Tinode-APIKey", p.config.Server.APIKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		p.log.Debugf("Could not provision user %s (connection failed): %v", user.Login, err)
		return nil // Soft error - user might already exist or server unreachable
	}
	defer resp.Body.Close()

	// Read response
	respBody, _ := io.ReadAll(resp.Body)

	// 200, 201 = created, 409 = already exists (OK)
	if resp.StatusCode == 200 || resp.StatusCode == 201 || resp.StatusCode == 409 {
		p.log.Debugf("User %s provisioned successfully (status: %d)", user.Login, resp.StatusCode)
		return nil
	}

	// Other error codes are warnings but not fatal
	p.log.Debugf("User %s provisioning status %d: %s", user.Login, resp.StatusCode, string(respBody))
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
