package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"n-manager/internal/apps"
	"n-manager/internal/ftp"
	"n-manager/internal/mysqladm"
	"n-manager/internal/nginx"
	"n-manager/internal/pkgmgr"
	"n-manager/internal/systemd"
	"n-manager/internal/validate"
)

func (s *Server) mysqlStatus(w http.ResponseWriter, r *http.Request)    { s.dbStatus(w, r, "mysql") }
func (s *Server) postgresStatus(w http.ResponseWriter, r *http.Request) { s.dbStatus(w, r, "postgres") }
func (s *Server) mysqlInstall(w http.ResponseWriter, r *http.Request)   { s.dbInstall(w, r, "mysql") }
func (s *Server) postgresInstall(w http.ResponseWriter, r *http.Request) {
	s.dbInstall(w, r, "postgres")
}
func (s *Server) mysqlService(w http.ResponseWriter, r *http.Request) { s.dbService(w, r, "mysql") }
func (s *Server) postgresService(w http.ResponseWriter, r *http.Request) {
	s.dbService(w, r, "postgres")
}
func (s *Server) mysqlLogs(w http.ResponseWriter, r *http.Request)    { s.dbLogs(w, r, "mysql") }
func (s *Server) postgresLogs(w http.ResponseWriter, r *http.Request) { s.dbLogs(w, r, "postgres") }

func (s *Server) mysqlAdmin(w http.ResponseWriter, r *http.Request) {
	view := mysqladm.Collect()
	if view.Error != "" && !s.dry() {
		ctx, cancel := s.opCtx(r)
		defer cancel()
		out, err := s.exec.Run(ctx, "mysql-list", nil, "")
		if err == nil {
			var parsed mysqladm.Admin
			if json.Unmarshal([]byte(out), &parsed) == nil {
				parsed.PHPMyAdmin = view.PHPMyAdmin
				view = parsed
			}
		}
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) mysqlSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MaxConnections int `json:"max_connections"`
		WaitTimeout    int `json:"wait_timeout"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	s.mysqlOp(w, r, "mysql-settings", []string{strconv.Itoa(body.MaxConnections), strconv.Itoa(body.WaitTimeout)}, "", "mysql.settings", "mysql")
}

func (s *Server) mysqlCreateDatabase(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	s.mysqlOp(w, r, "mysql-db-create", []string{body.Name}, "", "mysql.database.create", body.Name)
}

func (s *Server) mysqlDropDatabase(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	s.mysqlOp(w, r, "mysql-db-drop", []string{body.Name}, "", "mysql.database.drop", body.Name)
}

func (s *Server) mysqlCreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	s.mysqlOp(w, r, "mysql-user-create", []string{body.Name, body.Host}, body.Password, "mysql.user.create", body.Name+"@"+body.Host)
}

func (s *Server) mysqlDropUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Host    string `json:"host"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	s.mysqlOp(w, r, "mysql-user-drop", []string{body.Name, body.Host}, "", "mysql.user.drop", body.Name+"@"+body.Host)
}

func (s *Server) mysqlUserPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	s.mysqlOp(w, r, "mysql-user-password", []string{body.Name, body.Host}, body.Password, "mysql.user.password", body.Name+"@"+body.Host)
}

func (s *Server) mysqlGrant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string   `json:"name"`
		Host       string   `json:"host"`
		Database   string   `json:"database"`
		Privileges []string `json:"privileges"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	args := append([]string{body.Name, body.Host, body.Database}, body.Privileges...)
	s.mysqlOp(w, r, "mysql-grant", args, "", "mysql.grant", body.Name+"@"+body.Host+" "+body.Database)
}

func (s *Server) mysqlRevoke(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Database string `json:"database"`
		Confirm  bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	s.mysqlOp(w, r, "mysql-revoke", []string{body.Name, body.Host, body.Database}, "", "mysql.revoke", body.Name+"@"+body.Host+" "+body.Database)
}

func (s *Server) mysqlPHPMyAdminInstall(w http.ResponseWriter, r *http.Request) {
	mgr, err := pkgmgr.Detect()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mysqlOp(w, r, "phpmyadmin-install", []string{mgr.ID}, "", "mysql.phpmyadmin.install", "phpmyadmin")
}

func (s *Server) mysqlPHPMyAdmin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action  string `json:"action"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	op := "phpmyadmin-enable"
	if body.Action == "disable" {
		if !body.Confirm {
			writeErr(w, http.StatusBadRequest, "confirmation is required")
			return
		}
		op = "phpmyadmin-disable"
	} else if body.Action != "enable" {
		writeErr(w, http.StatusBadRequest, "invalid phpmyadmin request")
		return
	}
	s.mysqlOp(w, r, op, nil, "", "mysql.phpmyadmin."+body.Action, "phpmyadmin")
}

func (s *Server) mysqlOp(w http.ResponseWriter, r *http.Request, op string, args []string, stdin, action, target string) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, op, args, stdin)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	detail := ""
	if s.dry() {
		detail = "dry_run"
	}
	s.record(r, action, target, detail)
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) dbStatus(w http.ResponseWriter, r *http.Request, id string) {
	eng, err := apps.Database(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, eng)
}

func (s *Server) dbInstall(w http.ResponseWriter, r *http.Request, id string) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.InstallDatabase(ctx, s.exec, id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	detail := ""
	if s.dry() {
		detail = "dry_run"
	}
	s.record(r, "database.install", id, detail)
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) dbService(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Action  string `json:"action"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.Service(ctx, s.exec, id, body.Action, body.Confirm)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "confirmation") {
			status = http.StatusBadRequest
		}
		writeErr(w, status, err.Error())
		return
	}
	detail := body.Action
	if s.dry() {
		detail += " dry_run"
	}
	s.record(r, "database.service", id, detail)
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) dbLogs(w http.ResponseWriter, r *http.Request, id string) {
	text, err := apps.Logs(r.Context(), s.exec, id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"logs": text, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": text})
}

func (s *Server) nodeStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, apps.Node(r.Context()))
}

func (s *Server) nodeNVMInstall(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.InstallNVM(ctx, s.exec)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "nvm.install", "nvm", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nodeInstall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version string `json:"version"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.InstallNode(ctx, s.exec, body.Version)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "node.install", body.Version, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nodeRemove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version string `json:"version"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.RemoveNode(ctx, s.exec, body.Version, body.Confirm)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "node.uninstall", body.Version, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nodeDefault(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version string `json:"version"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.SetDefault(ctx, s.exec, body.Version)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "node.default", body.Version, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nodePackageInstall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.InstallGlobal(ctx, s.exec, body.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "npm.install", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) pm2Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, apps.PM2(r.Context()))
}

func (s *Server) pm2Install(w http.ResponseWriter, r *http.Request) {
	s.pm2Do(w, r, "install", "", "", false)
}

func (s *Server) pm2Action(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action  string `json:"action"`
		Name    string `json:"name"`
		Script  string `json:"script"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	s.pm2Do(w, r, body.Action, body.Name, body.Script, body.Confirm)
}

func (s *Server) pm2Logs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if err := validate.PM2Name(name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := s.runPM2(r, "logs", name, "")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"logs": out, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": out})
}

func (s *Server) pm2Do(w http.ResponseWriter, r *http.Request, action, name, script string, confirm bool) {
	if (action == "stop" || action == "delete") && !confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	if err := validate.PM2Operation(pm2ValidateArgs(action, name, script)); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	var (
		out string
		err error
	)
	if apps.ManagedNVM() {
		out, err = s.exec.Run(ctx, "pm2", pm2ValidateArgs(action, name, script), "")
	} else {
		out, err = apps.PM2Run(ctx, action, name, script)
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "pm2."+action, name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) runPM2(r *http.Request, action, name, script string) (string, error) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	if apps.ManagedNVM() {
		return s.exec.Run(ctx, "pm2", pm2ValidateArgs(action, name, script), "")
	}
	return apps.PM2Run(ctx, action, name, script)
}

func pm2ValidateArgs(action, name, script string) []string {
	switch action {
	case "install":
		return []string{"install"}
	case "start":
		if script != "" {
			return []string{"start", name, script}
		}
		return []string{"start", name}
	default:
		return []string{action, name}
	}
}

func (s *Server) nodePackageRemove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.RemoveGlobal(ctx, s.exec, body.Name, body.Confirm)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "npm.uninstall", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxStatus(w http.ResponseWriter, r *http.Request) {
	mgr, err := pkgmgr.Detect()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	installed, ver, _ := pkgmgr.Installed(mgr.ID, "nginx")
	st := systemd.Probe([]string{"nginx"})
	layout := ""
	if detected, err := nginx.Detect(""); err == nil {
		layout = detected.Kind
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"installed": installed,
		"version":   ver,
		"unit":      st.Unit,
		"active":    st.Active,
		"enabled":   st.Enabled,
		"exists":    st.Exists,
		"layout":    layout,
		"manager":   mgr.ID,
	})
}

func (s *Server) nginxInstall(w http.ResponseWriter, r *http.Request) {
	mgr, err := pkgmgr.Detect()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "pkg-install", []string{mgr.ID, "nginx"}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = s.exec.Run(ctx, "systemctl", []string{"enable", "nginx"}, "")
	s.record(r, "nginx.install", "nginx", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxService(w http.ResponseWriter, r *http.Request) {
	var body struct {
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
	out, err := systemd.Action(ctx, s.exec, body.Action, "nginx")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "nginx.service", "nginx", body.Action+" "+dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxSites(w http.ResponseWriter, r *http.Request) {
	layout, err := nginx.Detect("")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"sites": []nginx.Site{}, "error": err.Error()})
		return
	}
	sites, err := nginx.List(layout)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sites": sites, "layout": layout.Kind})
}

func (s *Server) nginxCreate(w http.ResponseWriter, r *http.Request) {
	s.nginxWrite(w, r, "")
}

func (s *Server) nginxUpdate(w http.ResponseWriter, r *http.Request) {
	s.nginxWrite(w, r, r.PathValue("name"))
}

func (s *Server) nginxWrite(w http.ResponseWriter, r *http.Request, name string) {
	var spec nginx.Spec
	if err := decode(r, &spec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if name != "" {
		spec.Name = name
	}
	content, err := nginx.Prepare(spec)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, err := json.Marshal(validate.SiteApply{Name: spec.Name, Content: content, Enabled: spec.Enabled})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "nginx-apply", nil, string(payload))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "nginx.site.write", spec.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxEnable(w http.ResponseWriter, r *http.Request) {
	s.nginxFlag(w, r, "nginx-enable")
}

func (s *Server) nginxDisable(w http.ResponseWriter, r *http.Request) {
	s.nginxFlag(w, r, "nginx-disable")
}

func (s *Server) nginxFlag(w http.ResponseWriter, r *http.Request, op string) {
	name := r.PathValue("name")
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, op, []string{name}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, op, name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &body); err != nil || !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	name := r.PathValue("name")
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "nginx-delete", []string{name}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "nginx.site.delete", name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxMain(w http.ResponseWriter, r *http.Request) {
	const path = "/etc/nginx/nginx.conf"
	b, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"path": path, "config": "", "error": err.Error()})
		return
	}
	if len(b) > 64*1024 {
		b = b[:64*1024]
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path, "config": string(b)})
}

func (s *Server) nginxPreview(w http.ResponseWriter, r *http.Request) {
	var spec nginx.Spec
	if err := decode(r, &spec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	content, err := nginx.Prepare(spec)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"config": content})
}

func (s *Server) nginxCerts(w http.ResponseWriter, r *http.Request) {
	installed, version := false, ""
	if mgr, err := pkgmgr.Detect(); err == nil {
		installed, version, _ = pkgmgr.Installed(mgr.ID, "certbot")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"certificates":    nginx.Certificates(),
		"certbot":         installed,
		"certbot_version": version,
	})
}

func (s *Server) nginxCertbotIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Site    string   `json:"site"`
		Email   string   `json:"email"`
		Domains []string `json:"domains"`
		Confirm bool     `json:"confirm"`
	}
	if err := decode(r, &body); err != nil || !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	if err := validate.Email(body.Email); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	domains := body.Domains
	var site nginx.Site
	haveSite := body.Site != ""
	if haveSite {
		if err := validate.SiteName(body.Site); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		layout, err := nginx.Detect("")
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		site, err = nginx.Read(layout, body.Site)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		domains = strings.Fields(site.Domains)
	}
	if err := validate.CertbotDomains(domains); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	mgr, err := pkgmgr.Detect()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	if installed, _, _ := pkgmgr.Installed(mgr.ID, "certbot"); !installed {
		if _, err := s.exec.Run(ctx, "pkg-install", []string{mgr.ID, "certbot"}, ""); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	content := ""
	if haveSite {
		updated, err := nginx.EnsureChallenge(site.Config)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.applySite(ctx, site.Name, updated, true); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		content = updated
	}
	if _, err := s.exec.Run(ctx, "certbot-issue", append([]string{body.Email}, domains...), ""); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if haveSite {
		cert := "/etc/letsencrypt/live/" + domains[0] + "/fullchain.pem"
		key := "/etc/letsencrypt/live/" + domains[0] + "/privkey.pem"
		final, err := nginx.UseCertificate(content, cert, key)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.applySite(ctx, site.Name, final, true); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	s.record(r, "nginx.certbot.issue", strings.Join(domains, " "), dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), "certificate requested"))
}

func (s *Server) nginxCertbotRenew(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &body); err != nil || !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "certbot-renew", nil, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if st := systemd.Probe([]string{"nginx"}); st.Exists {
		if reloaded, rerr := systemd.Action(ctx, s.exec, "reload", "nginx"); rerr == nil {
			out = strings.TrimSpace(out + "\n" + reloaded)
		}
	}
	s.record(r, "nginx.certbot.renew", "certbot", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) applySite(ctx context.Context, name, content string, enabled bool) error {
	payload, err := json.Marshal(validate.SiteApply{Name: name, Content: content, Enabled: enabled})
	if err != nil {
		return err
	}
	_, err = s.exec.Run(ctx, "nginx-apply", nil, string(payload))
	return err
}

func (s *Server) nginxCertPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Certificate string `json:"certificate"`
		Key         string `json:"key"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := validate.SiteName(body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validate.NginxCertificate(body.Certificate, body.Key); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, _ := json.Marshal(validate.NginxCert{Certificate: body.Certificate, Key: body.Key})
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "nginx-cert-put", []string{body.Name}, string(payload))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "nginx.cert.put", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxCertDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil || !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	if err := validate.SiteName(body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "nginx-cert-delete", []string{body.Name}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "nginx.cert.delete", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"access": s.readLog(r, "/var/log/nginx/access.log"),
		"error":  s.readLog(r, "/var/log/nginx/error.log"),
	})
}

func (s *Server) readLog(r *http.Request, path string) string {
	text, err := nginx.Tail(path, 200)
	if err == nil {
		return text
	}
	if s.dry() {
		return err.Error()
	}
	out, ferr := s.exec.Run(r.Context(), "read-file", []string{path}, "")
	if ferr != nil {
		return err.Error()
	}
	return out
}

func (s *Server) ftpStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ftp.Read())
}

func (s *Server) ftpInstall(w http.ResponseWriter, r *http.Request) {
	mgr, err := pkgmgr.Detect()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	payload, _ := json.Marshal(validate.FTPDefaults())
	var logs []string
	for _, step := range []struct {
		op, stdin string
		args      []string
	}{
		{op: "pkg-install", args: []string{mgr.ID, "vsftpd"}},
		{op: "ftp-apply", stdin: string(payload)},
		{op: "systemctl", args: []string{"enable", "vsftpd"}},
		{op: "systemctl", args: []string{"restart", "vsftpd"}},
	} {
		out, err := s.exec.Run(ctx, step.op, step.args, step.stdin)
		logs = append(logs, strings.TrimSpace(out))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	s.record(r, "ftp.install", "vsftpd", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), strings.Join(logs, "\n")))
}

func (s *Server) ftpSave(w http.ResponseWriter, r *http.Request) {
	var spec validate.FTPApply
	if err := decode(r, &spec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := validate.FTPConfig(spec); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, _ := json.Marshal(spec)
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "ftp-apply", nil, string(payload))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if ftp.Read().Exists && !s.dry() {
		if restarted, rerr := s.exec.Run(ctx, "systemctl", []string{"restart", "vsftpd"}, ""); rerr == nil {
			out = strings.TrimSpace(out + "\n" + restarted)
		}
	}
	s.record(r, "ftp.config", "vsftpd", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) ftpService(w http.ResponseWriter, r *http.Request) {
	var body struct {
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
	out, err := systemd.Action(ctx, s.exec, body.Action, "vsftpd")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "ftp.service", body.Action, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) ftpLogs(w http.ResponseWriter, r *http.Request) {
	text, err := systemd.Logs(r.Context(), s.exec, "vsftpd", 200)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"logs": text, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": text})
}

func (s *Server) ftpHomes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"paths": ftp.Homes()})
}

func (s *Server) ftpTransfers(w http.ResponseWriter, r *http.Request) {
	text := s.readLog(r, ftp.XferLog)
	writeJSON(w, http.StatusOK, map[string]string{"logs": text})
}

func (s *Server) ftpUserPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := validate.FTPName(body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validate.FTPPassword(body.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "ftp-user-password", []string{body.Name}, body.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "ftp.user.password", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) ftpUserLock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Lock    bool   `json:"lock"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Lock && !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	if err := validate.FTPName(body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	mode := "unlock"
	if body.Lock {
		mode = "lock"
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "ftp-user-lock", []string{body.Name, mode}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "ftp.user."+mode, body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) ftpUserWrite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Write bool   `json:"write"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := validate.FTPName(body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	mode := "no"
	if body.Write {
		mode = "yes"
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "ftp-user-write", []string{body.Name, mode}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "ftp.user.write", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) ftpUserAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Home     string `json:"home"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := validate.FTPUser(body.Name, body.Home); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validate.FTPPassword(body.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "ftp-user-add", []string{body.Name, body.Home}, body.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "ftp.user.add", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) ftpUserDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	if err := validate.FTPName(body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.exec.Run(ctx, "ftp-user-delete", []string{body.Name}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "ftp.user.delete", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func dryDetail(dry bool) string {
	if dry {
		return "dry_run"
	}
	return ""
}
