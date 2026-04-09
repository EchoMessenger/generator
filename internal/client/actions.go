package client

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// Actions provides high-level Tinode operations wrapper
type Actions struct {
	session *Session
	log     *logrus.Logger
}

// NewActions creates new actions wrapper
func NewActions(session *Session, log *logrus.Logger) *Actions {
	return &Actions{
		session: session,
		log:     log,
	}
}

// SubscribeToTopic subscribes to a topic
func (a *Actions) SubscribeToTopic(ctx context.Context, topic string) error {
	return a.session.Subscribe(ctx, topic)
}

// PublishMessage publishes a message to a topic
func (a *Actions) PublishMessage(ctx context.Context, topic string, content string) error {
	return a.session.Publish(ctx, topic, content)
}

// UnsubscribeFromTopic unsubscribes from a topic
func (a *Actions) UnsubscribeFromTopic(ctx context.Context, topic string) error {
	return a.session.Leave(ctx, topic, false)
}

// DeleteMessages deletes messages from a topic
func (a *Actions) DeleteMessages(ctx context.Context, topic string, ranges []DelRange, hard bool) error {
	if len(ranges) == 0 {
		return fmt.Errorf("no ranges specified")
	}

	return a.session.Delete(ctx, topic, ranges, hard)
}

// DeleteMessageRange is a convenience function to delete a single range
func (a *Actions) DeleteMessageRange(ctx context.Context, topic string, low, hi int) error {
	ranges := []DelRange{
		{Low: low, Hi: hi},
	}
	return a.DeleteMessages(ctx, topic, ranges, false)
}

// PublishBurst publishes multiple messages in quick succession
func (a *Actions) PublishBurst(ctx context.Context, topic string, count int, contentFn func(int) string) error {
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		content := contentFn(i)
		if err := a.PublishMessage(ctx, topic, content); err != nil {
			a.log.Warnf("Failed to publish message %d: %v", i, err)
			// Continue publishing despite errors
		}
	}

	return nil
}

// PublishAtRate publishes messages at a controlled rate (messages per minute)
func (a *Actions) PublishAtRate(ctx context.Context, topic string, count int, messagesPerMinute int, contentFn func(int) string) error {
	if messagesPerMinute <= 0 {
		messagesPerMinute = 60
	}

	// Calculate interval between messages
	intervalMs := (60 * 1000) / messagesPerMinute

	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		content := contentFn(i)
		if err := a.PublishMessage(ctx, topic, content); err != nil {
			a.log.Warnf("Failed to publish message %d: %v", i, err)
		}

		// Wait for next interval (except for last message)
		if i < count-1 {
			select {
			case <-time.After(time.Duration(intervalMs) * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return nil
}

// AttempLogin attempts to login and returns error if fails (useful for R1 brute force)
func (a *Actions) AttemptLogin(ctx context.Context, client *Client, username, password string) error {
	s := NewSession(client, username, password, a.log)
	return s.Connect(ctx)
}

// GetUserID returns authenticated user ID
func (a *Actions) GetUserID() string {
	return a.session.GetUID()
}

// GetToken returns authentication token
func (a *Actions) GetToken() string {
	return a.session.GetToken()
}

// GetSession returns underlying session
func (a *Actions) GetSession() *Session {
	return a.session
}

// Close closes the actions/session
func (a *Actions) Close() error {
	return a.session.Close()
}
