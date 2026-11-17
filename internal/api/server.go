package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"n-manager/internal/audit"
	"n-manager/internal/auth"
	"n-manager/internal/config"
	"n-manager/internal/monitor"
	"n-manager/internal/priv"
	"n-manager/internal/webui"
)

type ctxKey int

const csrfKey ctxKey = 1

type Server struct {
	cfg     *config.File
	db      *sql.DB
	auth    *auth.Manager
	audit   *audit.Logger
	monitor *monitor.Collector
	exec    priv.Executor
}

func New(cfg *config.File, database *sql.DB, collector *monitor.Collector, exec priv.Executor) *Server {
	snap := cfg.Snapshot()
	return &Server{
		cfg:     cfg,
		db:      database,
		auth:    auth.New(database, snap.Security.SessionSecret, snap.SessionTTL()),
		audit:   audit.New(database),
		monitor: collector,
		exec:    exec,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("GET /api/csrf", s.csrfToken)
	mux.HandleFunc("GET /api/system/info", s.systemInfo)
	mux.HandleFunc("GET /api/system/metrics", s.systemMetrics)
	mux.HandleFunc("GET /api/apps/mysql", s.mysqlStatus)
	mux.HandleFunc("POST /api/apps/mysql/install", s.mysqlInstall)
	mux.HandleFunc("POST /api/apps/mysql/service", s.mysqlService)
	mux.HandleFunc("GET /api/apps/mysql/logs", s.mysqlLogs)
	mux.HandleFunc("GET /api/apps/mysql/admin", s.mysqlAdmin)
	mux.HandleFunc("POST /api/apps/mysql/settings", s.mysqlSettings)
	mux.HandleFunc("POST /api/apps/mysql/databases", s.mysqlCreateDatabase)
	mux.HandleFunc("DELETE /api/apps/mysql/databases", s.mysqlDropDatabase)
	mux.HandleFunc("POST /api/apps/mysql/users", s.mysqlCreateUser)
	mux.HandleFunc("DELETE /api/apps/mysql/users", s.mysqlDropUser)
	mux.HandleFunc("PUT /api/apps/mysql/users/password", s.mysqlUserPassword)
	mux.HandleFunc("POST /api/apps/mysql/grants", s.mysqlGrant)
	mux.HandleFunc("DELETE /api/apps/mysql/grants", s.mysqlRevoke)
	mux.HandleFunc("POST /api/apps/mysql/phpmyadmin/install", s.mysqlPHPMyAdminInstall)
	mux.HandleFunc("POST /api/apps/mysql/phpmyadmin", s.mysqlPHPMyAdmin)
	mux.HandleFunc("GET /api/apps/postgres", s.postgresStatus)
	mux.HandleFunc("POST /api/apps/postgres/install", s.postgresInstall)
	mux.HandleFunc("POST /api/apps/postgres/service", s.postgresService)
	mux.HandleFunc("GET /api/apps/postgres/logs", s.postgresLogs)
	mux.HandleFunc("GET /api/apps/node", s.nodeStatus)
	mux.HandleFunc("POST /api/apps/node/nvm/install", s.nodeNVMInstall)
	mux.HandleFunc("POST /api/apps/node/versions", s.nodeInstall)
	mux.HandleFunc("POST /api/apps/node/versions/remove", s.nodeRemove)
	mux.HandleFunc("POST /api/apps/node/default", s.nodeDefault)
	mux.HandleFunc("POST /api/apps/node/packages", s.nodePackageInstall)
	mux.HandleFunc("POST /api/apps/node/packages/remove", s.nodePackageRemove)
	mux.HandleFunc("GET /api/apps/pm2", s.pm2Status)
	mux.HandleFunc("POST /api/apps/pm2/install", s.pm2Install)
	mux.HandleFunc("POST /api/apps/pm2/action", s.pm2Action)
	mux.HandleFunc("GET /api/apps/pm2/logs", s.pm2Logs)
	mux.HandleFunc("GET /api/ftp", s.ftpStatus)
	mux.HandleFunc("POST /api/ftp/install", s.ftpInstall)
	mux.HandleFunc("PUT /api/ftp", s.ftpSave)
	mux.HandleFunc("POST /api/ftp/service", s.ftpService)
	mux.HandleFunc("GET /api/ftp/logs", s.ftpLogs)
	mux.HandleFunc("GET /api/ftp/homes", s.ftpHomes)
	mux.HandleFunc("GET /api/ftp/transfers", s.ftpTransfers)
	mux.HandleFunc("POST /api/ftp/users", s.ftpUserAdd)
	mux.HandleFunc("DELETE /api/ftp/users", s.ftpUserDelete)
	mux.HandleFunc("PUT /api/ftp/users/password", s.ftpUserPassword)
	mux.HandleFunc("POST /api/ftp/users/lock", s.ftpUserLock)
	mux.HandleFunc("POST /api/ftp/users/write", s.ftpUserWrite)
	mux.HandleFunc("GET /api/nginx", s.nginxStatus)
	mux.HandleFunc("POST /api/nginx/install", s.nginxInstall)
	mux.HandleFunc("POST /api/nginx/service", s.nginxService)
	mux.HandleFunc("GET /api/nginx/sites", s.nginxSites)
	mux.HandleFunc("POST /api/nginx/sites", s.nginxCreate)
	mux.HandleFunc("PUT /api/nginx/sites/{name}", s.nginxUpdate)
	mux.HandleFunc("POST /api/nginx/sites/{name}/enable", s.nginxEnable)
	mux.HandleFunc("POST /api/nginx/sites/{name}/disable", s.nginxDisable)
	mux.HandleFunc("DELETE /api/nginx/sites/{name}", s.nginxDelete)
	mux.HandleFunc("GET /api/nginx/logs", s.nginxLogs)
	mux.HandleFunc("GET /api/nginx/config", s.nginxMain)
	mux.HandleFunc("POST /api/nginx/preview", s.nginxPreview)
	mux.HandleFunc("GET /api/nginx/certificates", s.nginxCerts)
	mux.HandleFunc("POST /api/nginx/certificates/issue", s.nginxCertbotIssue)
	mux.HandleFunc("POST /api/nginx/certificates/renew", s.nginxCertbotRenew)
	mux.HandleFunc("POST /api/nginx/certificates", s.nginxCertPut)
	mux.HandleFunc("DELETE /api/nginx/certificates", s.nginxCertDelete)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("POST /api/settings/password", s.changePassword)
	mux.HandleFunc("GET /api/settings/database", s.databaseSettings)
	mux.HandleFunc("GET /api/audit", s.auditLog)
	mux.Handle("/", s.spa())
	return s.headers(s.authenticate(s.checkCSRF(mux)))
}

func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("X-Robots-Tag", "noindex")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/login" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		token, err := s.auth.Session(cookie.Value)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), csrfKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) checkCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/login" {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("X-CSRF-Token")
		want, _ := r.Context().Value(csrfKey).(string)
		if got == "" || want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			writeErr(w, http.StatusForbidden, "csrf token is missing or invalid")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) spa() http.Handler {
	fsys, err := webui.FS()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "ui unavailable", http.StatusInternalServerError)
		})
	}
	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(fsys, name); err != nil {
			clone := r.Clone(r.Context())
			clone.URL.Path = "/"
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, clone)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) record(r *http.Request, action, target, detail string) {
	_ = s.audit.Record("admin", action, target, detail, clientIP(r))
}

func (s *Server) opCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 10*time.Minute)
}

func (s *Server) dry() bool {
	return s.cfg.Snapshot().Security.DryRun
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) cookieSecure(r *http.Request) bool {
	switch s.cfg.Snapshot().Security.CookieSecure {
	case "true":
		return true
	case "false":
		return false
	default:
		return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, value string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.cookieSecure(r),
		Expires:  exp,
		MaxAge:   int(time.Until(exp).Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func result(dry bool, output string) map[string]any {
	msg := "completed"
	if dry {
		msg = "dry run: no system changes were made"
	}
	return map[string]any{"ok": true, "dry_run": dry, "message": msg, "output": output}
}
