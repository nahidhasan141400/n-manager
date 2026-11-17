package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) debugLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := s.trace.Recent(limit, r.URL.Query().Get("level"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "log could not be read")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) clearDebugLogs(w http.ResponseWriter, r *http.Request) {
	if err := s.trace.Clear(); err != nil {
		writeErr(w, http.StatusInternalServerError, "log could not be cleared")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) logErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap := &capture{ResponseWriter: w}
		next.ServeHTTP(cap, r)
		if cap.status < 400 || !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/logs" {
			return
		}
		s.trace.Note("error", s.serverLabel(r), "request", r.Method+" "+r.URL.Path, strconv.Itoa(cap.status)+" "+errorText(cap.buf.String()))
	})
}

func (s *Server) serverLabel(r *http.Request) string {
	id := strings.TrimSpace(r.Header.Get("X-Server-Id"))
	if id == "" || s.servers == nil {
		return ""
	}
	rec, err := s.servers.Get(id)
	if err != nil {
		return ""
	}
	if rec.Name != "" {
		return rec.Name
	}
	return rec.Username + "@" + rec.Host
}

type capture struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (w *capture) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *capture) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.buf.Len() < 512 {
		remain := 512 - w.buf.Len()
		if len(p) > remain {
			w.buf.Write(p[:remain])
		} else {
			w.buf.Write(p)
		}
	}
	return w.ResponseWriter.Write(p)
}

func errorText(body string) string {
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(body), &payload) == nil && payload.Error != "" {
		return payload.Error
	}
	text := strings.TrimSpace(body)
	if text == "" {
		return "request failed"
	}
	return text
}
