package utils

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// EventLogger provides structured event logging for audit trail
type EventLogger struct {
	log  *logrus.Logger
	file io.Writer
	mu   sync.Mutex
}

// Event represents a logged event
type Event struct {
	Timestamp time.Time              `json:"timestamp"`
	EventType string                 `json:"event_type"`
	Scenario  string                 `json:"scenario"`
	User      string                 `json:"user,omitempty"`
	Topic     string                 `json:"topic,omitempty"`
	Action    string                 `json:"action"`
	Result    string                 `json:"result"` // success, failure, pending
	Message   string                 `json:"message,omitempty"`
	ErrorCode int                    `json:"error_code,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewEventLogger creates a new event logger
func NewEventLogger(log *logrus.Logger, filePath string) (*EventLogger, error) {
	el := &EventLogger{
		log: log,
	}

	// Set up file output if specified
	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		el.file = f
	}

	return el, nil
}

// LogEvent logs a structured event
func (el *EventLogger) LogEvent(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	el.mu.Lock()
	defer el.mu.Unlock()

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		el.log.Warnf("Failed to marshal event: %v", err)
		return
	}

	// Write to file if configured
	if el.file != nil {
		el.file.Write(append(data, '\n'))
	}

	// Log to logrus
	el.log.WithFields(logrus.Fields{
		"scenario": event.Scenario,
		"user":     event.User,
		"action":   event.Action,
		"result":   event.Result,
	}).Infof("[%s] %s", event.EventType, event.Message)
}

// LogScenarioStart logs start of scenario
func (el *EventLogger) LogScenarioStart(scenarioName, description string) {
	el.LogEvent(Event{
		EventType: "scenario_start",
		Scenario:  scenarioName,
		Action:    "start",
		Result:    "pending",
		Message:   description,
	})
}

// LogScenarioComplete logs completion of scenario
func (el *EventLogger) LogScenarioComplete(scenarioName string, success bool, duration time.Duration, messageCount int64, errorCount int64) {
	result := "success"
	if !success {
		result = "failure"
	}

	el.LogEvent(Event{
		EventType: "scenario_complete",
		Scenario:  scenarioName,
		Action:    "complete",
		Result:    result,
		Message:   "Scenario completed",
		Metadata: map[string]interface{}{
			"duration_ms":   duration.Milliseconds(),
			"message_count": messageCount,
			"error_count":   errorCount,
		},
	})
}

// LogAction logs a user action (publish, subscribe, delete, etc)
func (el *EventLogger) LogAction(scenario, user, topic, action, result string, metadata map[string]interface{}) {
	el.LogEvent(Event{
		Timestamp: time.Now(),
		EventType: "action",
		Scenario:  scenario,
		User:      user,
		Topic:     topic,
		Action:    action,
		Result:    result,
		Metadata:  metadata,
	})
}

// LogAuthAttempt logs authentication attempt
func (el *EventLogger) LogAuthAttempt(user, result string, errorCode int) {
	el.LogEvent(Event{
		Timestamp: time.Now(),
		EventType: "auth",
		Action:    "login",
		User:      user,
		Result:    result,
		ErrorCode: errorCode,
	})
}

// LogMessage logs a message event
func (el *EventLogger) LogMessage(scenario, user, topic, action string, success bool) {
	result := "success"
	if !success {
		result = "failure"
	}

	el.LogEvent(Event{
		Timestamp: time.Now(),
		EventType: "message",
		Scenario:  scenario,
		User:      user,
		Topic:     topic,
		Action:    action,
		Result:    result,
	})
}

// Close closes the event logger file
func (el *EventLogger) Close() error {
	if f, ok := el.file.(*os.File); ok {
		return f.Close()
	}
	return nil
}
