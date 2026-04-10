package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// Client represents a WebSocket connection to Tinode server
type Client struct {
	url    string
	apiKey string
	log    *logrus.Logger
	conn   *websocket.Conn
	mu     sync.RWMutex
	send   chan interface{}
	recv   chan interface{}
	done   chan struct{}
	msgID  int64 // Atomic counter for message IDs
	closed int32 // Atomic flag for closed state
}

// Message types as defined in Tinode protocol
type ClientMessage struct {
	Hi    *HiMessage    `json:"hi,omitempty"`
	Acc   *AccMessage   `json:"acc,omitempty"`
	Login *LoginMessage `json:"login,omitempty"`
	Sub   *SubMessage   `json:"sub,omitempty"`
	Leave *LeaveMessage `json:"leave,omitempty"`
	Pub   *PubMessage   `json:"pub,omitempty"`
	Get   *GetMessage   `json:"get,omitempty"`
	Set   *SetMessage   `json:"set,omitempty"`
	Del   *DelMessage   `json:"del,omitempty"`
	Note  *NoteMessage  `json:"note,omitempty"`
}

// Server response message
type ServerMessage struct {
	Ctrl *CtrlMessage `json:"ctrl,omitempty"`
	Data *DataMessage `json:"data,omitempty"`
	Meta *MetaMessage `json:"meta,omitempty"`
	Pres *PresMessage `json:"pres,omitempty"`
	Info *InfoMessage `json:"info,omitempty"`
}

// Wire protocol message types
type HiMessage struct {
	ID       string `json:"id,omitempty"`
	Ver      string `json:"ver"`
	UA       string `json:"ua,omitempty"`
	Dev      string `json:"dev,omitempty"`
	Lang     string `json:"lang,omitempty"`
	Platform string `json:"platf,omitempty"`
	Bkg      bool   `json:"bkg,omitempty"`
}

type LoginMessage struct {
	ID     string `json:"id,omitempty"`
	Scheme string `json:"scheme"`
	Secret []byte `json:"secret"`
}

type SubMessage struct {
	ID    string `json:"id,omitempty"`
	Topic string `json:"topic"`
}

type LeaveMessage struct {
	ID    string `json:"id,omitempty"`
	Topic string `json:"topic"`
	Unsub bool   `json:"unsub,omitempty"`
}

type PubMessage struct {
	ID      string      `json:"id,omitempty"`
	Topic   string      `json:"topic"`
	NoEcho  bool        `json:"noecho,omitempty"`
	Content interface{} `json:"content"`
}

type GetMessage struct {
	ID    string `json:"id,omitempty"`
	Topic string `json:"topic"`
	What  string `json:"what"`
}

type SetMessage struct {
	ID    string      `json:"id,omitempty"`
	Topic string      `json:"topic"`
	Desc  interface{} `json:"desc,omitempty"`
}

type DelMessage struct {
	ID     string     `json:"id,omitempty"`
	Topic  string     `json:"topic"`
	What   string     `json:"what"`
	Hard   bool       `json:"hard,omitempty"`
	DelSeq []DelRange `json:"delseq,omitempty"`
}

type DelRange struct {
	Low int `json:"low"`
	Hi  int `json:"hi,omitempty"`
}

type NoteMessage struct {
	Topic string `json:"topic"`
	What  string `json:"what"`
	Seq   int    `json:"seq,omitempty"`
	Event string `json:"event,omitempty"`
}

type AccMessage struct {
	ID     string                 `json:"id,omitempty"`
	User   string                 `json:"user,omitempty"`
	Scheme string                 `json:"scheme,omitempty"`
	Secret []byte                 `json:"secret,omitempty"`
	Login  bool                   `json:"login,omitempty"`
	Passwd string                 `json:"passwd,omitempty"` // For account creation
	Public map[string]interface{} `json:"public,omitempty"` // For account creation
}

// Server response types
type CtrlMessage struct {
	ID     string                 `json:"id,omitempty"`
	Topic  string                 `json:"topic,omitempty"`
	Code   int                    `json:"code"`
	Text   string                 `json:"text"`
	Params map[string]interface{} `json:"params,omitempty"`
}

type DataMessage struct {
	Topic   string      `json:"topic"`
	From    string      `json:"from,omitempty"`
	Head    interface{} `json:"head,omitempty"`
	Content interface{} `json:"content"`
	Seq     int         `json:"seq"`
	Ts      string      `json:"ts"`
}

type MetaMessage struct {
	ID    string `json:"id,omitempty"`
	Topic string `json:"topic,omitempty"`
}

type PresMessage struct {
	Topic string `json:"topic"`
	Src   string `json:"src"`
	What  string `json:"what"`
}

type InfoMessage struct {
	Topic string      `json:"topic"`
	From  string      `json:"from"`
	What  string      `json:"what"`
	Seq   int         `json:"seq,omitempty"`
	Head  interface{} `json:"head,omitempty"`
}

// NewClient creates a new Tinode client
func NewClient(url, apiKey string, log *logrus.Logger) *Client {
	return &Client{
		url:    url,
		apiKey: apiKey,
		log:    log,
		send:   make(chan interface{}, 100),
		recv:   make(chan interface{}, 100),
		done:   make(chan struct{}),
		msgID:  0,
	}
}

// Connect establishes WebSocket connection to server
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return fmt.Errorf("already connected")
	}

	// Add API key to WebSocket URL
	url := c.url
	if c.apiKey != "" {
		sep := "?"
		if len(url) > 0 && url[len(url)-1] == '?' {
			sep = ""
		}
		url = fmt.Sprintf("%s%sapikey=%s", url, sep, c.apiKey)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout:  10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Allow self-signed certificates for wss://
		},
	}

	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.log.Debugf("Connected to %s", c.url)

	// Start read and write loops
	go c.readLoop()
	go c.writeLoop()

	return nil
}

// Close closes the WebSocket connection
func (c *Client) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return fmt.Errorf("already closed")
	}

	close(c.done)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// IsClosed returns true if client is closed
func (c *Client) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) != 0
}

// Send sends a message to server (non-blocking)
func (c *Client) Send(msg interface{}) error {
	if c.IsClosed() {
		return fmt.Errorf("client is closed")
	}

	select {
	case c.send <- msg:
		return nil
	case <-c.done:
		return fmt.Errorf("client is closed")
	default:
		return fmt.Errorf("send queue full")
	}
}

// SendSync sends a message and blocks until sent or context cancelled
func (c *Client) SendSync(ctx context.Context, msg interface{}) error {
	if c.IsClosed() {
		return fmt.Errorf("client is closed")
	}

	select {
	case c.send <- msg:
		return nil
	case <-c.done:
		return fmt.Errorf("client is closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Recv receives a message from server (blocks until message or close)
func (c *Client) Recv() (interface{}, error) {
	select {
	case msg := <-c.recv:
		return msg, nil
	case <-c.done:
		return nil, fmt.Errorf("client is closed")
	}
}

// RecvTimeout receives a message with timeout
func (c *Client) RecvTimeout(timeout time.Duration) (interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.RecvCtx(ctx)
}

// RecvCtx receives a message with context
func (c *Client) RecvCtx(ctx context.Context) (interface{}, error) {
	select {
	case msg := <-c.recv:
		return msg, nil
	case <-c.done:
		return nil, fmt.Errorf("client is closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// NextMsgID returns next message ID
func (c *Client) NextMsgID() string {
	id := atomic.AddInt64(&c.msgID, 1)
	return fmt.Sprintf("%d", id)
}

// readLoop continuously reads messages from WebSocket
func (c *Client) readLoop() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
		close(c.recv)
	}()

	c.conn.SetReadLimit(1024 * 1024) // 1MB limit
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for !c.IsClosed() {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.log.Warnf("WebSocket read error: %v", err)
			}
			return
		}

		var msg ServerMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			c.log.Warnf("Failed to unmarshal message: %v", err)
			continue
		}

		select {
		case c.recv <- &msg:
		case <-c.done:
			return
		}
	}
}

// writeLoop continuously sends messages to WebSocket
func (c *Client) writeLoop() {
	ticker := time.NewTicker(54 * time.Second) // Ping every ~54 seconds (keepalive)
	defer func() {
		ticker.Stop()
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}

			if err := c.writeMessage(msg); err != nil {
				c.log.Warnf("Write error: %v", err)
				return
			}

		case <-ticker.C:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn != nil {
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					c.log.Warnf("Ping error: %v", err)
					return
				}
			}

		case <-c.done:
			return
		}
	}
}

// writeMessage marshals and writes a message to WebSocket
func (c *Client) writeMessage(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("no connection")
	}

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, data)
}
