package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"n-manager/internal/priv"
	"n-manager/internal/remote"
	"n-manager/internal/validate"
)

type rejectExec struct{ err error }

func (e rejectExec) Run(context.Context, string, []string, string) (string, error) {
	if e.err == nil {
		return "", remote.ErrNoServer
	}
	return "", e.err
}

type dialExec struct {
	s  *Server
	id string
}

func (e dialExec) Run(ctx context.Context, op string, args []string, stdin string) (string, error) {
	sys, err := e.s.pool.Open(ctx, e.s.servers, e.id)
	if err != nil {
		return "", err
	}
	return priv.Perform(remote.With(ctx, sys), op, args, stdin)
}

func (s *Server) execFor(r *http.Request) priv.Executor {
	id := strings.TrimSpace(r.Header.Get("X-Server-Id"))
	if id == "" {
		return rejectExec{err: remote.ErrNoServer}
	}
	if _, err := s.servers.Get(id); err != nil {
		return rejectExec{err: err}
	}
	if s.dry() {
		return priv.DryRun{}
	}
	return dialExec{s: s, id: id}
}

func (s *Server) withMachine(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, bool) {
	ctx, cancel, err := s.machine(r)
	if err != nil {
		s.writeOpErr(w, err)
		return nil, func() {}, false
	}
	return ctx, cancel, true
}

func (s *Server) machine(r *http.Request) (context.Context, context.CancelFunc, error) {
	id := strings.TrimSpace(r.Header.Get("X-Server-Id"))
	if id == "" {
		return nil, func() {}, remote.ErrNoServer
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	sys, err := s.pool.Open(ctx, s.servers, id)
	if err != nil {
		cancel()
		return nil, func() {}, err
	}
	return remote.With(ctx, sys), cancel, nil
}

func (s *Server) writeOpErr(w http.ResponseWriter, err error) {
	var change *remote.KeyChange
	if errors.As(err, &change) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": change.Error(), "needs_trust": true, "fingerprint": change.Fingerprint,
		})
		return
	}
	status := http.StatusBadRequest
	if errors.Is(err, remote.ErrNoServer) {
		status = http.StatusConflict
	}
	writeErr(w, status, err.Error())
}

func (s *Server) listServers(w http.ResponseWriter, r *http.Request) {
	items, err := s.servers.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": items})
}

func (s *Server) createServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Port == 0 {
		body.Port = 22
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		body.Name = body.Username + "@" + body.Host
	}
	if err := validate.ServerLogin(body.Name, body.Host, body.Port, body.Username, body.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Password == "" {
		writeErr(w, http.StatusBadRequest, "ssh password must be 1 to 128 characters")
		return
	}
	item, err := s.servers.Create(body.Name, strings.TrimSpace(body.Host), body.Port, strings.TrimSpace(body.Username), body.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "server.create", item.Name, item.Host)
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Port == 0 {
		body.Port = 22
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		body.Name = body.Username + "@" + body.Host
	}
	if err := validate.ServerLogin(body.Name, body.Host, body.Port, body.Username, body.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.servers.Update(id, body.Name, strings.TrimSpace(body.Host), body.Port, strings.TrimSpace(body.Username), body.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.pool.Close(id)
	s.record(r, "server.update", item.Name, item.Host)
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &body); err != nil || !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	if err := s.servers.Delete(id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.pool.Close(id)
	s.record(r, "server.delete", id, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) testServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	sys, err := s.pool.Open(ctx, s.servers, id)
	if err != nil {
		s.writeOpErr(w, err)
		return
	}
	if _, err := sys.Run(ctx, "true", nil, ""); err != nil {
		s.writeOpErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "trusted": true})
}

func (s *Server) trustServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &body); err != nil || !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	if err := s.pool.Trust(s.servers, id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "server.trust", id, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "trusted": true})
}
