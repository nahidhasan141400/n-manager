package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"n-manager/internal/pkgmgr"
	"n-manager/internal/remote"
	"n-manager/internal/systemd"
	"n-manager/internal/validate"
)

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	units := []string{"nginx", "apache2", "httpd", "mysql", "mariadb", "mysqld", "postgresql", "vsftpd"}
	for _, pattern := range []string{"/usr/lib/systemd/system/php*-fpm.service", "/lib/systemd/system/php*-fpm.service"} {
		found, _ := remote.Glob(ctx, pattern)
		for _, path := range found {
			unit := strings.TrimSuffix(filepath.Base(path), ".service")
			if validate.AllowedUnit(unit) {
				units = append(units, unit)
			}
		}
	}
	forced := map[string]bool{}
	if mgr, err := pkgmgr.Detect(ctx); err == nil {
		for _, pkg := range []string{"apache2", "httpd", "nginx"} {
			installed, _, qerr := pkgmgr.Installed(ctx, mgr.ID, pkg)
			if qerr == nil && installed && validate.AllowedUnit(pkg) {
				units = append(units, pkg)
				forced[pkg] = true
			}
		}
	}
	seen := map[string]bool{}
	list := []systemd.Status{}
	for _, unit := range units {
		if seen[unit] {
			continue
		}
		seen[unit] = true
		st := systemd.Probe(ctx, []string{unit})
		if !st.Exists && forced[unit] {
			st = systemd.Status{Unit: unit, Active: "not-found", Enabled: "unknown", Exists: true}
		}
		if st.Exists {
			list = append(list, st)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": list})
}

func (s *Server) serviceLogs(w http.ResponseWriter, r *http.Request) {
	unit := r.URL.Query().Get("unit")
	if !validate.AllowedUnit(unit) {
		writeErr(w, http.StatusBadRequest, "invalid service request")
		return
	}
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	out, err := systemd.Logs(ctx, s.execFor(r), unit, 80)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": out})
}

func (s *Server) serviceAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Unit    string `json:"unit"`
		Action  string `json:"action"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if (body.Action == "stop" || body.Action == "disable") && !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	if body.Unit == "nginx" && (body.Action == "start" || body.Action == "restart") {
		_, _ = s.execFor(r).Run(ctx, "systemctl", []string{"stop", "apache2"}, "")
		_, _ = s.execFor(r).Run(ctx, "systemctl", []string{"stop", "httpd"}, "")
		hideApacheWelcome(ctx)
	}
	out, err := systemd.Action(ctx, s.execFor(r), body.Action, body.Unit)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "service."+body.Action, body.Unit, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}
