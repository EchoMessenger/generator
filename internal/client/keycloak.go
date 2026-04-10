package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"
)

// KeycloakClient handles JWT token acquisition from Keycloak
type KeycloakClient struct {
	url        string // Keycloak base URL
	realm      string // Realm name
	clientID   string // Client ID
	httpClient *http.Client
	log        *logrus.Entry
}

// KeycloakTokenResponse is the response from Keycloak token endpoint
type KeycloakTokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	IDToken          string `json:"id_token"`
	NotBeforePolicy  int    `json:"not-before-policy"`
	SessionState     string `json:"session_state"`
	Scope            string `json:"scope"`
}

// NewKeycloakClient creates a new Keycloak client
func NewKeycloakClient(keycloakURL, realm, clientID string, log *logrus.Entry) *KeycloakClient {
	return &KeycloakClient{
		url:      keycloakURL,
		realm:    realm,
		clientID: clientID,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		log: log,
	}
}

// GetToken obtains a JWT access token from Keycloak using username/password
func (kc *KeycloakClient) GetToken(username, password string) (string, error) {
	if kc.url == "" || kc.realm == "" {
		return "", fmt.Errorf("keycloak_url or keycloak_realm not configured")
	}

	// Build token endpoint URL
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", kc.url, kc.realm)

	// Prepare request body
	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", kc.clientID)
	data.Set("username", username)
	data.Set("password", password)

	kc.log.Debugf("Requesting Keycloak token for user %s from %s", username, tokenURL)

	// Send request
	resp, err := kc.httpClient.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("failed to request Keycloak token: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Keycloak response: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var tokenResp KeycloakTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse Keycloak response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("keycloak response missing access_token")
	}

	kc.log.Debugf("Successfully obtained JWT token for user %s", username)
	return tokenResp.AccessToken, nil
}
