package api

import (
	"errors"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
	"n-manager/internal/auth"
	"n-manager/internal/db"
	"n-manager/internal/version"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	snap := s.cfg.Snapshot()
	cookie, csrf, exp, err := s.auth.Login(body.Password, snap.Security.AdminPasswordHash, clientIP(r), r.UserAgent())
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrRateLimited):
			writeErr(w, http.StatusTooManyRequests, "too many login attempts")
		default:
			writeErr(w, http.StatusUnauthorized, "incorrect password")
		}
		return
	}
	s.setSessionCookie(w, r, cookie, exp)
	s.record(r, "login", "admin", "")
	writeJSON(w, http.StatusOK, map[string]any{"csrf_token": csrf, "expires_at": exp.UTC().Format(time.RFC3339)})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.CookieName); err == nil {
		s.auth.Logout(cookie.Value)
	}
	clearSessionCookie(w, r, s.cookieSecure(r))
	s.record(r, "logout", "admin", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": "admin", "version": version.Version})
}

func (s *Server) csrfToken(w http.ResponseWriter, r *http.Request) {
	token, _ := r.Context().Value(csrfKey).(string)
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": token})
}

func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.monitor.Info()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) systemMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.monitor.Snapshot())
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	snap := s.cfg.Snapshot()
	st, err := db.StatusOf(s.db, snap.Database.Path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":             version.Version,
		"host":                snap.Server.Host,
		"port":                snap.Server.Port,
		"tls_cert":            snap.Server.TLSCert,
		"tls_key":             snap.Server.TLSKey,
		"refresh_interval":    snap.Monitoring.RefreshInterval,
		"session_ttl_minutes": snap.Security.SessionTTLMinutes,
		"dry_run":             snap.Security.DryRun,
		"database":            st,
	})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshInterval   *int    `json:"refresh_interval"`
		Host              *string `json:"host"`
		Port              *int    `json:"port"`
		TLSCert           *string `json:"tls_cert"`
		TLSKey            *string `json:"tls_key"`
		SessionTTLMinutes *int    `json:"session_ttl_minutes"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	restart := false
	if body.RefreshInterval != nil {
		if err := s.cfg.SetRefreshInterval(*body.RefreshInterval); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.monitor.SetInterval(time.Duration(*body.RefreshInterval) * time.Second)
	}
	if body.SessionTTLMinutes != nil {
		if err := s.cfg.SetSessionTTL(*body.SessionTTLMinutes); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.auth.SetTTL(time.Duration(*body.SessionTTLMinutes) * time.Minute)
	}
	if body.Host != nil || body.Port != nil || body.TLSCert != nil || body.TLSKey != nil {
		snap := s.cfg.Snapshot()
		host, port, cert, key := snap.Server.Host, snap.Server.Port, snap.Server.TLSCert, snap.Server.TLSKey
		if body.Host != nil {
			host = *body.Host
		}
		if body.Port != nil {
			port = *body.Port
		}
		if body.TLSCert != nil {
			cert = *body.TLSCert
		}
		if body.TLSKey != nil {
			key = *body.TLSKey
		}
		changed, err := s.cfg.SetBind(host, port, cert, key)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		restart = changed
	}
	s.record(r, "settings.update", "config", "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restart_required": restart})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current_password"`
		Next    string `json:"new_password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	snap := s.cfg.Snapshot()
	if err := bcrypt.CompareHashAndPassword([]byte(snap.Security.AdminPasswordHash), []byte(body.Current)); err != nil {
		writeErr(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	hash, err := auth.HashPassword(body.Next)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cfg.SetPasswordHash(hash); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.record(r, "password.change", "admin", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) databaseSettings(w http.ResponseWriter, r *http.Request) {
	snap := s.cfg.Snapshot()
	st, err := db.StatusOf(s.db, snap.Database.Path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) auditLog(w http.ResponseWriter, r *http.Request) {
	entries, err := s.audit.Recent(50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
