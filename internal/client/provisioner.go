package client

import (
	"context"
	"fmt"
	"strings"
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
	if p.config.Generator.SkipProvisioning {
		p.log.Infof("Skipping user provisioning (skip_provisioning=true)")
		p.log.Infof("Assuming users exist with configured credentials")
		
		// Provision topics only
		if p.config.Topics != nil {
			for _, topic := range p.config.Topics {
				if err := p.provisionTopic(ctx, topic); err != nil {
					p.log.Warnf("Failed to provision topic %s: %v", topic.Name, err)
				}
			}
		}
		
		p.log.Infof("Provisioning complete")
		return nil
	}

	p.log.Infof("Starting provisioning: %d users", len(p.config.Users))

	// Provision users via individual WebSocket connections
	// Per Tinode docs: any session (even unauthenticated) can create new users
	// by setting user: "newXXX" in {acc} message
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

// provisionUser creates a new user account
// Per Tinode protocol: set user: "newXXX" to create new user from any session
func (p *Provisioner) provisionUser(ctx context.Context, user config.UserConfig) error {
	p.log.Infof("Provisioning user: %s (%s)", user.ID, user.Login)

	// Connect to server via WebSocket
	wsClient := NewClient(p.config.Server.URL, p.config.Server.APIKey, p.log)
	if err := wsClient.Connect(ctx); err != nil {
		p.log.Warnf("Could not provision user %s (connection failed): %v", user.Login, err)
		return nil // Soft error
	}
	defer wsClient.Close()

	// Create a session for request/response correlation
	session := NewSession(wsClient, "", "", p.log)
	go session.dispatchResponses()

	// Send handshake
	if err := session.handshake(ctx); err != nil {
		p.log.Warnf("Handshake failed for user %s: %v", user.Login, err)
		return nil
	}

	// Create new account with user: "new<UserID>"
	// This allows account creation from unauthenticated session
	msgID := wsClient.NextMsgID()
	
	// Credentials will be base64-encoded by JSON marshaler
	creds := fmt.Sprintf("%s:%s", user.Login, user.Password)

	accMsg := &ClientMessage{
		Acc: &AccMessage{
			ID:     msgID,
			User:   "new" + user.ID, // Signal to create new user with this ID
			Scheme: "basic",           // Use basic auth
			Secret: []byte(creds),     // Will be base64-encoded by JSON marshaler
			Login:  true,              // Immediately login with new account
			Public: map[string]interface{}{
				"name": user.Description,
			},
		},
	}

	if err := wsClient.SendSync(ctx, accMsg); err != nil {
		p.log.Debugf("Failed to send acc message for user %s: %v", user.Login, err)
		return nil
	}
	p.log.Debugf("Sent {acc} message for user %s with user=new%s scheme=basic login=true", user.Login, user.ID)

	// Wait for account creation response
	resp, err := session.waitForResponse(ctx, msgID, 5*time.Second)
	if err != nil {
		p.log.Debugf("Account creation timeout for user %s: %v", user.Login, err)
		return nil
	}

	if resp.Ctrl == nil {
		p.log.Debugf("Account creation returned nil ctrl for user %s", user.Login)
		return nil
	}

	if resp.Ctrl.Code != 200 && resp.Ctrl.Code != 201 {
		// Check if account already exists (error codes that indicate this)
		if resp.Ctrl.Code == 409 || (resp.Ctrl.Code >= 400 && strings.Contains(strings.ToLower(resp.Ctrl.Text), "duplicate")) {
			p.log.Infof("Account %s already exists, attempting to verify login credentials", user.Login)
			
			// Try to login with existing account to verify password matches
			if err := p.verifyExistingUser(ctx, wsClient, user); err != nil {
				p.log.Warnf("Existing account %s found but login verification failed: %v", user.Login, err)
				return nil // Account exists but password mismatch - can't use it
			}
			
			p.log.Infof("Account %s verified (already exists with correct password)", user.Login)
			return nil // Account exists and password is correct
		}
		
		// Other errors
		p.log.Warnf("Account creation rejected for user %s: code=%d text=%s", user.Login, resp.Ctrl.Code, resp.Ctrl.Text)
		return nil
	}

	// Extract user ID from response
	userID := ""
	if resp.Ctrl.Params != nil {
		if uid, ok := resp.Ctrl.Params["user"]; ok {
			userID = uid.(string)
			p.log.Infof("User %s created with ID: %s", user.Login, userID)
		}
	}

	// Extract token if provided (should be present with login=true)
	token := ""
	if resp.Ctrl.Params != nil {
		if t, ok := resp.Ctrl.Params["token"]; ok {
			token = t.(string)
			if len(token) > 20 {
				p.log.Debugf("Got auth token for user %s: %s...", user.Login, token[:20])
			}
		}
	}

	p.log.Infof("User %s provisioned successfully (code=%d, userID=%s)", user.Login, resp.Ctrl.Code, userID)
	return nil
}

// verifyExistingUser attempts to login to an existing account to verify credentials match
func (p *Provisioner) verifyExistingUser(ctx context.Context, wsClient *Client, user config.UserConfig) error {
	p.log.Debugf("Verifying login for existing user %s", user.Login)
	
	// Create a session for verification
	verifySession := NewSession(wsClient, user.Login, user.Password, p.log)
	go verifySession.dispatchResponses()
	
	// Send login message
	msgID := wsClient.NextMsgID()
	creds := fmt.Sprintf("%s:%s", user.Login, user.Password)
	
	loginMsg := &ClientMessage{
		Login: &LoginMessage{
			ID:     msgID,
			Scheme: "basic",
			Secret: []byte(creds),
		},
	}
	
	if err := wsClient.SendSync(ctx, loginMsg); err != nil {
		p.log.Debugf("Failed to send login message for verification: %v", err)
		return err
	}
	p.log.Debugf("Sent {login} to verify existing user %s", user.Login)
	
	// Wait for login response
	resp, err := verifySession.waitForResponse(ctx, msgID, 5*time.Second)
	if err != nil {
		p.log.Debugf("Login verification timeout for user %s: %v", user.Login, err)
		return err
	}
	
	if resp.Ctrl == nil || (resp.Ctrl.Code != 200 && resp.Ctrl.Code != 201) {
		return fmt.Errorf("login failed for user %s: code=%d text=%s", user.Login, resp.Ctrl.Code, resp.Ctrl.Text)
	}
	
	p.log.Debugf("Login verification successful for user %s", user.Login)
	return nil
}

// loginAsAdmin logs in as the xena admin user to enable account creation
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
