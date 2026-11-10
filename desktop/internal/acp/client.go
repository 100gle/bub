package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
)

// ErrClosed is reported by calls made or in flight when the connection ends,
// as when the agent process exits.
var ErrClosed = errors.New("acp: agent connection closed")

// maxLine bounds one JSON-RPC message; a longer line ends the connection
// rather than growing without limit.
const maxLine = 16 << 20

// Client speaks ACP over an io.ReadWriteCloser. It reads messages in a
// goroutine of its own: responses go to the call waiting for them, and
// session/update notifications go to the update handler.
//
// A Client handles one transport. Start builds one around a spawned agent
// process; NewClient takes any connection, as tests take in-memory pipes.
type Client struct {
	conn      io.ReadWriteCloser
	wmu       sync.Mutex // serializes writes: one JSON-RPC message per line
	nextID    atomic.Int64
	mu        sync.Mutex
	pending   map[int64]chan *wireMessage
	onUpdate  func(Update)
	onClose   func(error)
	logger    *log.Logger
	closed    chan struct{}
	closeOnce sync.Once
	readErr   error
}

// NewClient starts a client on conn. The connection is read until it ends,
// which closes the client; Close closes it sooner.
func NewClient(conn io.ReadWriteCloser) *Client {
	c := &Client{
		conn:    conn,
		pending: make(map[int64]chan *wireMessage),
		logger:  log.New(io.Discard, "", 0),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// SetLogger sets where the client logs malformed lines and other trouble;
// the default discards them. It must be called before messages arrive.
func (c *Client) SetLogger(l *log.Logger) {
	if l == nil {
		l = log.New(io.Discard, "", 0)
	}
	c.mu.Lock()
	c.logger = l
	c.mu.Unlock()
}

// SetUpdateHandler sets what receives session/update notifications. The
// handler runs on the client's read goroutine: run UI work on the main thread
// (mygo.Window.Update) rather than here.
func (c *Client) SetUpdateHandler(fn func(Update)) {
	c.mu.Lock()
	c.onUpdate = fn
	c.mu.Unlock()
}

// SetCloseHandler sets what hears that the connection ended, once, with the
// reason.
func (c *Client) SetCloseHandler(fn func(error)) {
	c.mu.Lock()
	c.onClose = fn
	c.mu.Unlock()
}

// Close ends the connection: the agent sees its standard input close and is
// expected to exit. Calls in flight fail with ErrClosed.
func (c *Client) Close() error {
	err := c.conn.Close()
	c.fail(ErrClosed)
	return err
}

// Call sends a request and waits for its response, or for ctx to end. out,
// when not nil, receives the result.
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	id, ch, err := c.dispatchRequest(method, params)
	if err != nil {
		return err
	}
	select {
	case msg, ok := <-ch:
		if !ok {
			return c.closedError(method)
		}
		if msg.Error != nil {
			return msg.Error
		}
		if out != nil && len(msg.Result) > 0 {
			if err := json.Unmarshal(msg.Result, out); err != nil {
				return fmt.Errorf("acp: %s: decode result: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		c.forget(id)
		return fmt.Errorf("acp: %s: %w", method, ctx.Err())
	case <-c.closed:
		return c.closedError(method)
	}
}

// notify sends a notification: a message that expects no response.
func (c *Client) notify(method string, params any) error {
	return c.writeJSON(notification{JSONRPC: "2.0", Method: method, Params: params})
}

func (c *Client) dispatchRequest(method string, params any) (int64, chan *wireMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan *wireMessage, 1)
	c.mu.Lock()
	if c.pending == nil { // already closed
		c.mu.Unlock()
		return 0, nil, c.closedError(method)
	}
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.writeJSON(request{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.forget(id)
		return 0, nil, fmt.Errorf("acp: %s: %w", method, err)
	}
	return id, ch, nil
}

func (c *Client) forget(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) closedError(method string) error {
	c.mu.Lock()
	err := c.readErr
	c.mu.Unlock()
	if err == nil {
		err = ErrClosed
	}
	return fmt.Errorf("acp: %s: %w", method, err)
}

func (c *Client) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := c.conn.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func (c *Client) logf(format string, args ...any) {
	c.mu.Lock()
	logger := c.logger
	c.mu.Unlock()
	logger.Printf(format, args...)
}

func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		c.handleLine(line)
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	c.fail(err)
}

func (c *Client) handleLine(line []byte) {
	var msg wireMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		c.logf("acp: skipping malformed line: %v", err)
		return
	}
	hasID := len(msg.ID) > 0 && string(msg.ID) != "null"
	switch {
	case msg.Method != "" && hasID: // a request from the agent
		c.answerRequest(msg)
	case msg.Method != "": // a notification
		c.handleNotification(msg)
	case hasID: // a response
		c.handleResponse(msg)
	default:
		c.logf("acp: skipping message with neither method nor id")
	}
}

// answerRequest replies to a request the client does not implement with
// "method not found", so that an agent waiting for the answer carries on
// instead of hanging. Bub asks for none of them today; fs/*, terminal/* and
// session/request_permission would arrive here if it did.
func (c *Client) answerRequest(msg wireMessage) {
	c.logf("acp: answering unimplemented request %s with -32601", msg.Method)
	resp := response{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Error: &RPCError{
			Code:    ErrCodeMethodNotFound,
			Message: msg.Method + " is not implemented by bub-desktop",
		},
	}
	if err := c.writeJSON(resp); err != nil {
		c.logf("acp: answering %s: %v", msg.Method, err)
	}
}

func (c *Client) handleNotification(msg wireMessage) {
	if msg.Method != "session/update" {
		c.logf("acp: ignoring notification %s", msg.Method)
		return
	}
	update, err := parseSessionNotification(msg.Params)
	if err != nil {
		c.logf("acp: skipping session/update: %v", err)
		return
	}
	c.mu.Lock()
	handler := c.onUpdate
	c.mu.Unlock()
	if handler != nil {
		handler(update)
	}
}

func (c *Client) handleResponse(msg wireMessage) {
	var id int64
	if err := json.Unmarshal(msg.ID, &id); err != nil {
		c.logf("acp: skipping response with unusable id %s", msg.ID)
		return
	}
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch == nil {
		c.logf("acp: response for unknown request %d", id)
		return
	}
	ch <- &msg
}

// fail ends the client once, with the reason the connection ended.
func (c *Client) fail(err error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		switch {
		case err == nil, errors.Is(err, io.EOF), errors.Is(err, ErrClosed):
			c.readErr = ErrClosed
		default:
			c.readErr = fmt.Errorf("acp: connection ended: %w", err)
		}
		pending := c.pending
		c.pending = nil
		readErr := c.readErr
		handler := c.onClose
		c.mu.Unlock()
		for _, ch := range pending {
			close(ch)
		}
		close(c.closed)
		if handler != nil {
			handler(readErr)
		}
	})
}
