package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Session represents an authenticated Tinode session
type Session struct {
	client   *Client
	uid      string // Authenticated user ID
	token    string // Authentication token
	username string
	password string
	log      *logrus.Logger

	// Message correlation
	mu        sync.RWMutex
	responses map[string]chan *ServerMessage // id -> response channel
}

// NewSession creates a new session
func NewSession(client *Client, username, password string, log *logrus.Logger) *Session {
	return &Session{
		client:    client,
		username:  username,
		password:  password,
		log:       log,
		responses: make(map[string]chan *ServerMessage),
	}
}

// Connect performs handshake and authentication
func (s *Session) Connect(ctx context.Context) error {
	// Start response dispatcher BEFORE handshake (it reads responses from client)
	go s.dispatchResponses()

	// Step 1: Handshake
	if err := s.handshake(ctx); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	s.log.Debugf("Handshake successful")

	// Step 2: Login with basic auth
	if err := s.login(ctx); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	s.log.Infof("Authenticated as %s (uid: %s)", s.username, s.uid)

	return nil
}

// handshake sends {hi} message and waits for response
func (s *Session) handshake(ctx context.Context) error {
	msgID := s.client.NextMsgID()
	s.log.Debugf("Starting handshake with message ID: %s", msgID)

	msg := &ClientMessage{
		Hi: &HiMessage{
			ID:   msgID,
			Ver:  "0.19",
			UA:   "Generator/1.0",
			Lang: "en-US",
		},
	}

	if err := s.client.SendSync(ctx, msg); err != nil {
		return err
	}
	s.log.Debugf("Sent {hi} message, waiting for response...")

	// Wait for {ctrl code=200 or 201}
	resp, err := s.waitForResponse(ctx, msgID, 5*time.Second)
	if err != nil {
		return err
	}

	if resp.Ctrl == nil {
		return fmt.Errorf("expected {ctrl} response, got something else")
	}

	// Accept both 200 (OK) and 201 (Created) as successful handshake responses
	if resp.Ctrl.Code != 200 && resp.Ctrl.Code != 201 {
		return fmt.Errorf("handshake failed: code %d, text: %s", resp.Ctrl.Code, resp.Ctrl.Text)
	}

	s.log.Debugf("Handshake complete: received {ctrl code=%d}", resp.Ctrl.Code)
	return nil
}

// login sends {login scheme="basic"} and waits for token
func (s *Session) login(ctx context.Context) error {
	msgID := s.client.NextMsgID()

	// Encode credentials: username:password in base64
	creds := fmt.Sprintf("%s:%s", s.username, s.password)
	secret := base64.StdEncoding.EncodeToString([]byte(creds))
	s.log.Debugf("Login attempt: username=%s, scheme=basic", s.username)

	msg := &ClientMessage{
		Login: &LoginMessage{
			ID:     msgID,
			Scheme: "basic",
			Secret: []byte(secret),
		},
	}

	if err := s.client.SendSync(ctx, msg); err != nil {
		return err
	}
	s.log.Debugf("Sent {login} message with ID: %s", msgID)

	// Wait for {ctrl code=200} with token
	resp, err := s.waitForResponse(ctx, msgID, 5*time.Second)
	if err != nil {
		return err
	}

	if resp.Ctrl == nil {
		return fmt.Errorf("expected {ctrl} response, got something else")
	}

	if resp.Ctrl.Code != 200 {
		return fmt.Errorf("login failed: code %d, text: %s", resp.Ctrl.Code, resp.Ctrl.Text)
	}

	// Extract user ID and token from params
	if resp.Ctrl.Params == nil {
		return fmt.Errorf("no params in login response")
	}

	if uid, ok := resp.Ctrl.Params["user"]; ok {
		s.uid = uid.(string)
	} else {
		return fmt.Errorf("no user ID in response")
	}

	if token, ok := resp.Ctrl.Params["token"]; ok {
		s.token = token.(string)
	} else {
		return fmt.Errorf("no token in response")
	}

	return nil
}

// Subscribe subscribes to a topic
func (s *Session) Subscribe(ctx context.Context, topic string) error {
	msgID := s.client.NextMsgID()

	msg := &ClientMessage{
		Sub: &SubMessage{
			ID:    msgID,
			Topic: topic,
		},
	}

	if err := s.client.SendSync(ctx, msg); err != nil {
		return err
	}

	resp, err := s.waitForResponse(ctx, msgID, 10*time.Second)
	if err != nil {
		return err
	}

	if resp.Ctrl == nil {
		return fmt.Errorf("expected {ctrl} response")
	}

	if resp.Ctrl.Code != 200 {
		return fmt.Errorf("subscribe failed: code %d, text: %s", resp.Ctrl.Code, resp.Ctrl.Text)
	}

	s.log.Debugf("Subscribed to topic %s", topic)
	return nil
}

// Publish publishes a message to a topic
func (s *Session) Publish(ctx context.Context, topic, content string) error {
	msgID := s.client.NextMsgID()

	msg := &ClientMessage{
		Pub: &PubMessage{
			ID:      msgID,
			Topic:   topic,
			Content: content,
		},
	}

	if err := s.client.SendSync(ctx, msg); err != nil {
		return err
	}

	resp, err := s.waitForResponse(ctx, msgID, 10*time.Second)
	if err != nil {
		return err
	}

	if resp.Ctrl == nil {
		return fmt.Errorf("expected {ctrl} response")
	}

	if resp.Ctrl.Code != 200 {
		return fmt.Errorf("publish failed: code %d, text: %s", resp.Ctrl.Code, resp.Ctrl.Text)
	}

	return nil
}

// Leave unsubscribes from a topic
func (s *Session) Leave(ctx context.Context, topic string, unsub bool) error {
	msgID := s.client.NextMsgID()

	msg := &ClientMessage{
		Leave: &LeaveMessage{
			ID:    msgID,
			Topic: topic,
			Unsub: unsub,
		},
	}

	if err := s.client.SendSync(ctx, msg); err != nil {
		return err
	}

	resp, err := s.waitForResponse(ctx, msgID, 10*time.Second)
	if err != nil {
		return err
	}

	if resp.Ctrl == nil {
		return fmt.Errorf("expected {ctrl} response")
	}

	if resp.Ctrl.Code != 200 {
		return fmt.Errorf("leave failed: code %d, text: %s", resp.Ctrl.Code, resp.Ctrl.Text)
	}

	s.log.Debugf("Left topic %s (unsub=%v)", topic, unsub)
	return nil
}

// Delete deletes messages from a topic
func (s *Session) Delete(ctx context.Context, topic string, delseq []DelRange, hard bool) error {
	msgID := s.client.NextMsgID()

	msg := &ClientMessage{
		Del: &DelMessage{
			ID:     msgID,
			Topic:  topic,
			What:   "msg",
			Hard:   hard,
			DelSeq: delseq,
		},
	}

	if err := s.client.SendSync(ctx, msg); err != nil {
		return err
	}

	resp, err := s.waitForResponse(ctx, msgID, 10*time.Second)
	if err != nil {
		return err
	}

	if resp.Ctrl == nil {
		return fmt.Errorf("expected {ctrl} response")
	}

	if resp.Ctrl.Code != 200 {
		return fmt.Errorf("delete failed: code %d, text: %s", resp.Ctrl.Code, resp.Ctrl.Text)
	}

	s.log.Debugf("Deleted messages from topic %s", topic)
	return nil
}

// LoginToken logs in using token (subsequent logins)
func (s *Session) LoginToken(ctx context.Context, token string) error {
	msgID := s.client.NextMsgID()

	secret := base64.StdEncoding.EncodeToString([]byte(token))

	msg := &ClientMessage{
		Login: &LoginMessage{
			ID:     msgID,
			Scheme: "token",
			Secret: []byte(secret),
		},
	}

	if err := s.client.SendSync(ctx, msg); err != nil {
		return err
	}

	resp, err := s.waitForResponse(ctx, msgID, 5*time.Second)
	if err != nil {
		return err
	}

	if resp.Ctrl == nil {
		return fmt.Errorf("expected {ctrl} response")
	}

	if resp.Ctrl.Code != 200 {
		return fmt.Errorf("token login failed: code %d, text: %s", resp.Ctrl.Code, resp.Ctrl.Text)
	}

	return nil
}

// GetUID returns authenticated user ID
func (s *Session) GetUID() string {
	return s.uid
}

// GetToken returns authentication token
func (s *Session) GetToken() string {
	return s.token
}

// Close closes the session
func (s *Session) Close() error {
	return s.client.Close()
}

// waitForResponse waits for response with matching message ID
func (s *Session) waitForResponse(ctx context.Context, msgID string, timeout time.Duration) (*ServerMessage, error) {
	s.mu.Lock()
	ch := make(chan *ServerMessage, 1)
	s.responses[msgID] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.responses, msgID)
		s.mu.Unlock()
	}()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case resp := <-ch:
		return resp, nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("response timeout for message %s", msgID)
	}
}

// dispatchResponses reads from client and dispatches to waiting goroutines
func (s *Session) dispatchResponses() {
	s.log.Debugf("Response dispatcher started")
	for {
		msg, err := s.client.Recv()
		if err != nil {
			if !s.client.IsClosed() {
				s.log.Warnf("Recv error: %v", err)
			}
			return
		}

		srvMsg, ok := msg.(*ServerMessage)
		if !ok {
			s.log.Warnf("Unexpected message type: %T", msg)
			continue
		}

		// Extract message ID from response
		var msgID string
		if srvMsg.Ctrl != nil {
			msgID = srvMsg.Ctrl.ID
			s.log.Debugf("Received {ctrl} response: id=%s, code=%d", msgID, srvMsg.Ctrl.Code)
		} else if srvMsg.Meta != nil {
			msgID = srvMsg.Meta.ID
			s.log.Debugf("Received {meta} response: id=%s", msgID)
		}

		if msgID == "" {
			s.log.Debugf("Message without ID, ignoring")
			continue
		}

		// Find waiting goroutine
		s.mu.RLock()
		ch, ok := s.responses[msgID]
		s.mu.RUnlock()

		if ok {
			select {
			case ch <- srvMsg:
				s.log.Debugf("Dispatched response for message %s", msgID)
			case <-time.After(100 * time.Millisecond):
				s.log.Warnf("Response channel for %s was not consumed", msgID)
			}
		} else {
			// Unexpected response, log it
			s.log.Debugf("Received response for unknown message ID: %s", msgID)
		}
	}
}
