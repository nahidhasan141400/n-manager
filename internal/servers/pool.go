package servers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"n-manager/internal/remote"
)

// Pool keeps SSH connections and host keys waiting for confirmation.
type Pool struct {
	mu      sync.Mutex
	live    map[string]*remote.SSH
	pending map[string][]byte
	flight  map[string]*dialFlight
	trace   remote.Tracer
}

type dialFlight struct {
	done   chan struct{}
	client *remote.SSH
	err    error
}

func NewPool() *Pool {
	return &Pool{live: map[string]*remote.SSH{}, pending: map[string][]byte{}, flight: map[string]*dialFlight{}}
}

// SetTracer receives connection notes. It is set once before requests are served.
func (p *Pool) SetTracer(t remote.Tracer) {
	p.trace = t
}

func (p *Pool) note(level, server, kind, message, detail string) {
	if p.trace != nil {
		p.trace.Note(level, server, kind, message, detail)
	}
}

func (p *Pool) Open(ctx context.Context, store *Store, id string) (*remote.SSH, error) {
	rec, err := store.Get(id)
	if err != nil {
		return nil, err
	}
	server := rec.Name
	if server == "" {
		server = rec.Username + "@" + rec.Host
	}
	p.mu.Lock()
	if client := p.live[id]; client != nil {
		p.mu.Unlock()
		client.SetTrace(p.trace, server)
		if _, err := client.Run(ctx, "true", nil, ""); err == nil {
			return client, nil
		}
		if ctx.Err() == nil {
			p.note("info", server, "ssh", "connection dropped", rec.Host)
		}
		p.mu.Lock()
		if p.live[id] == client {
			delete(p.live, id)
		}
		p.mu.Unlock()
		_ = client.Close()
		p.mu.Lock()
	}
	if flight := p.flight[id]; flight != nil {
		p.mu.Unlock()
		select {
		case <-flight.done:
			return flight.client, flight.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	flight := &dialFlight{done: make(chan struct{})}
	p.flight[id] = flight
	p.mu.Unlock()

	client, presented, err := remote.Dial(ctx, remote.Endpoint{
		Host: rec.Host, Port: rec.Port, User: rec.Username, Password: rec.Password, HostKey: rec.HostKey,
	})
	if err != nil {
		var change *remote.KeyChange
		if errors.As(err, &change) {
			p.mu.Lock()
			p.pending[id] = change.Key
			p.mu.Unlock()
			p.note("error", server, "ssh", "host key changed", change.Fingerprint)
		} else if ctx.Err() == nil {
			detail := err.Error()
			if rec.Password != "" {
				detail = strings.ReplaceAll(detail, rec.Password, "")
			}
			p.note("error", server, "ssh", "login failed", detail)
		}
	} else if len(rec.HostKey) == 0 && len(presented) > 0 {
		if err = store.SetHostKey(id, presented); err != nil {
			client.Close()
			client = nil
			p.note("error", server, "ssh", "host key was not saved", err.Error())
		}
	}
	if err == nil {
		client.SetTrace(p.trace, server)
		port := rec.Port
		if port == 0 {
			port = 22
		}
		p.note("info", server, "ssh", "connected", fmt.Sprintf("%s@%s:%d", rec.Username, rec.Host, port))
	}
	p.mu.Lock()
	if err == nil {
		if old := p.live[id]; old != nil && old != client {
			_ = old.Close()
		}
		p.live[id] = client
		delete(p.pending, id)
	}
	flight.client = client
	flight.err = err
	delete(p.flight, id)
	close(flight.done)
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (p *Pool) Trust(store *Store, id string) error {
	p.mu.Lock()
	key := append([]byte(nil), p.pending[id]...)
	p.mu.Unlock()
	if len(key) == 0 {
		return errNoPending
	}
	if err := store.SetHostKey(id, key); err != nil {
		return err
	}
	p.Close(id)
	return nil
}

func (p *Pool) Close(id string) {
	p.mu.Lock()
	client := p.live[id]
	delete(p.live, id)
	delete(p.pending, id)
	p.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
}

var errNoPending = errString("confirm the host key from the test result")

type errString string

func (e errString) Error() string { return string(e) }
