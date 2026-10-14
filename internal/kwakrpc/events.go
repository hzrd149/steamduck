package kwakrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
)

// Event is one events.subscribe notification. Raw holds every field; the
// common ones are decoded for convenience.
type Event struct {
	Type     string `json:"type"`
	Time     string `json:"time"`
	WindowID string `json:"window_id,omitempty"`
	Address  string `json:"address,omitempty"`
	Name     string `json:"name,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	Dropped  int    `json:"dropped,omitempty"`

	Raw json.RawMessage `json:"-"`
}

// Subscription is an open event stream on its own connection.
type Subscription struct {
	conn net.Conn
	C    <-chan Event
	err  error
	done chan struct{}
}

// Subscribe opens a dedicated connection and subscribes to types (all when
// empty). Events arrive on the returned channel, which closes when the
// stream ends; Err then reports why.
func (c *Client) Subscribe(ctx context.Context, types ...string) (*Subscription, error) {
	conn, err := dial(ctx, c.path)
	if err != nil {
		return nil, err
	}
	params := map[string]any{}
	if len(types) > 0 {
		params["types"] = types
	}
	frame, _ := json.Marshal(request{JSONRPC: "2.0", Method: "events.subscribe", Params: params, ID: 1})
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if _, err := conn.Write(append(frame, '\n')); err != nil {
		conn.Close()
		return nil, err
	}
	r := bufio.NewReaderSize(conn, 64<<10)
	resp, err := readFrame(r)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if resp.Error != nil {
		conn.Close()
		return nil, resp.Error
	}
	// a subscribed connection has no read deadline on the server side
	_ = conn.SetDeadline(timeZero)

	ch := make(chan Event, 64)
	s := &Subscription{conn: conn, C: ch, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer close(ch)
		for {
			f, err := readFrame(r)
			if err != nil {
				s.err = err
				return
			}
			if f.Method != "event" {
				continue
			}
			var ev Event
			if json.Unmarshal(f.Params, &ev) != nil {
				continue
			}
			ev.Raw = f.Params
			ch <- ev
		}
	}()
	return s, nil
}

// Close ends the subscription.
func (s *Subscription) Close() error {
	err := s.conn.Close()
	<-s.done
	return err
}

// Err is why the stream ended, after C has closed.
func (s *Subscription) Err() error {
	if errors.Is(s.err, net.ErrClosed) {
		return nil
	}
	return s.err
}
