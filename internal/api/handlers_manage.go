package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"n-manager/internal/apps"
	"n-manager/internal/ftp"
	"n-manager/internal/mysqladm"
	"n-manager/internal/nginx"
	"n-manager/internal/pkgmgr"
	"n-manager/internal/remote"
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
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	view := mysqladm.Collect(ctx, "nginx")
	if view.Error != "" && !s.dry() {
		ctx, cancel := s.opCtx(r)
		defer cancel()
		out, err := s.execFor(r).Run(ctx, "mysql-list", []string{"nginx"}, "")
		if err == nil {
			var parsed mysqladm.Admin
			if json.Unmarshal([]byte(out), &parsed) == nil {
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

func (s *Server) mysqlExportDatabase(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if err := validate.MySQLDatabase(name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, "mysql-db-export", []string{name}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "mysql.database.export", name, "")
	w.Header().Set("Content-Type", "application/sql; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+".sql"))
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, out)
}

func (s *Server) mysqlImportDatabase(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, validate.MaxMySQLDump+1<<20)
	if err := r.ParseMultipartForm(validate.MaxMySQLDump); err != nil {
		writeErr(w, http.StatusBadRequest, "SQL file must be 32 MB or smaller")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if r.FormValue("confirm") != "true" {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "choose a SQL file")
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, validate.MaxMySQLDump+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read the SQL file")
		return
	}
	if err := validate.MySQLDump(string(body)); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mysqlOp(w, r, "mysql-db-import", []string{name}, string(body), "mysql.database.import", name)
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

func (s *Server) mysqlOp(w http.ResponseWriter, r *http.Request, op string, args []string, stdin, action, target string) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, op, args, stdin)
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
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	eng, err := apps.Database(ctx, id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, eng)
}

func (s *Server) dbInstall(w http.ResponseWriter, r *http.Request, id string) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.InstallDatabase(ctx, s.execFor(r), id)
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
	out, err := apps.Service(ctx, s.execFor(r), id, body.Action, body.Confirm)
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
	text, err := apps.Logs(r.Context(), s.execFor(r), id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"logs": text, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": text})
}

func (s *Server) nodeStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	writeJSON(w, http.StatusOK, apps.Node(ctx))
}

func (s *Server) nodeNVMInstall(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.InstallNVM(ctx, s.execFor(r))
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
	out, err := apps.InstallNode(ctx, s.execFor(r), body.Version)
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
	out, err := apps.RemoveNode(ctx, s.execFor(r), body.Version, body.Confirm)
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
	out, err := apps.SetDefault(ctx, s.execFor(r), body.Version)
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
	out, err := apps.InstallGlobal(ctx, s.execFor(r), body.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "npm.install", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) pm2Status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	writeJSON(w, http.StatusOK, apps.PM2(ctx))
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
	if apps.ManagedNVM(ctx) {
		out, err = s.execFor(r).Run(ctx, "pm2", pm2ValidateArgs(action, name, script), "")
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
	if apps.ManagedNVM(ctx) {
		return s.execFor(r).Run(ctx, "pm2", pm2ValidateArgs(action, name, script), "")
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
	out, err := apps.RemoveGlobal(ctx, s.execFor(r), body.Name, body.Confirm)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "npm.uninstall", body.Name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	mgr, err := pkgmgr.Detect(ctx)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	installed, ver, _ := pkgmgr.Installed(ctx, mgr.ID, "nginx")
	st := systemd.Probe(ctx, []string{"nginx"})
	layout := ""
	if detected, err := nginx.Detect(ctx, ""); err == nil {
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
	readCtx, readCancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer readCancel()
	mgr, err := pkgmgr.Detect(readCtx)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, "pkg-install", []string{mgr.ID, "nginx"}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = s.execFor(r).Run(ctx, "systemctl", []string{"enable", "nginx"}, "")
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
	if body.Action == "start" || body.Action == "restart" {
		_, _ = s.execFor(r).Run(ctx, "systemctl", []string{"stop", "apache2"}, "")
		_, _ = s.execFor(r).Run(ctx, "systemctl", []string{"stop", "httpd"}, "")
		hideApacheWelcome(ctx)
	}
	out, err := systemd.Action(ctx, s.execFor(r), body.Action, "nginx")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "nginx.service", "nginx", body.Action+" "+dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func hideApacheWelcome(ctx context.Context) {
	const page = "/var/www/html/index.html"
	body, err := remote.ReadFile(ctx, page)
	if err != nil || !bytes.Contains(body, []byte("Apache2 Ubuntu Default Page")) {
		return
	}
	if _, err := remote.Stat(ctx, "/var/www/html/index.nginx-debian.html"); err != nil {
		return
	}
	_ = remote.Rename(ctx, page, "/var/www/html/index.html.apache")
}

func (s *Server) nginxSites(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	layout, err := nginx.Detect(ctx, "")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"sites": []nginx.Site{}, "error": err.Error()})
		return
	}
	sites, err := nginx.List(ctx, layout)
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
	out, err := s.execFor(r).Run(ctx, "nginx-apply", nil, string(payload))
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
	out, err := s.execFor(r).Run(ctx, op, []string{name}, "")
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
	out, err := s.execFor(r).Run(ctx, "nginx-delete", []string{name}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "nginx.site.delete", name, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) nginxMain(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	const path = "/etc/nginx/nginx.conf"
	b, err := remote.ReadFile(ctx, path)
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
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	installed, version := false, ""
	if mgr, err := pkgmgr.Detect(ctx); err == nil {
		installed, version, _ = pkgmgr.Installed(ctx, mgr.ID, "certbot")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"certificates":    nginx.Certificates(ctx),
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
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	if haveSite {
		if err := validate.SiteName(body.Site); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		layout, err := nginx.Detect(ctx, "")
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		site, err = nginx.Read(ctx, layout, body.Site)
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
	mgr, err := pkgmgr.Detect(ctx)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if installed, _, _ := pkgmgr.Installed(ctx, mgr.ID, "certbot"); !installed {
		if _, err := s.execFor(r).Run(ctx, "pkg-install", []string{mgr.ID, "certbot"}, ""); err != nil {
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
		if err := s.applySite(ctx, r, site.Name, updated, true); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		content = updated
	}
	if _, err := s.execFor(r).Run(ctx, "certbot-issue", append([]string{body.Email}, domains...), ""); err != nil {
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
		if err := s.applySite(ctx, r, site.Name, final, true); err != nil {
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
	out, err := s.execFor(r).Run(ctx, "certbot-renew", nil, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if st := systemd.Probe(ctx, []string{"nginx"}); st.Exists {
		if reloaded, rerr := systemd.Action(ctx, s.execFor(r), "reload", "nginx"); rerr == nil {
			out = strings.TrimSpace(out + "\n" + reloaded)
		}
	}
	s.record(r, "nginx.certbot.renew", "certbot", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) applySite(ctx context.Context, r *http.Request, name, content string, enabled bool) error {
	payload, err := json.Marshal(validate.SiteApply{Name: name, Content: content, Enabled: enabled})
	if err != nil {
		return err
	}
	_, err = s.execFor(r).Run(ctx, "nginx-apply", nil, string(payload))
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
	out, err := s.execFor(r).Run(ctx, "nginx-cert-put", []string{body.Name}, string(payload))
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
	out, err := s.execFor(r).Run(ctx, "nginx-cert-delete", []string{body.Name}, "")
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
	text, err := nginx.Tail(r.Context(), path, 200)
	if err == nil {
		return text
	}
	if s.dry() {
		return err.Error()
	}
	out, ferr := s.execFor(r).Run(r.Context(), "read-file", []string{path}, "")
	if ferr != nil {
		return err.Error()
	}
	return out
}

func (s *Server) ftpStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	writeJSON(w, http.StatusOK, ftp.Read(ctx))
}

func (s *Server) ftpInstall(w http.ResponseWriter, r *http.Request) {
	readCtx, readCancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer readCancel()
	mgr, err := pkgmgr.Detect(readCtx)
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
		out, err := s.execFor(r).Run(ctx, step.op, step.args, step.stdin)
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
	out, err := s.execFor(r).Run(ctx, "ftp-apply", nil, string(payload))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if ftp.Read(ctx).Exists && !s.dry() {
		if restarted, rerr := s.execFor(r).Run(ctx, "systemctl", []string{"restart", "vsftpd"}, ""); rerr == nil {
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
	out, err := systemd.Action(ctx, s.execFor(r), body.Action, "vsftpd")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "ftp.service", body.Action, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) ftpLogs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	text, err := systemd.Logs(ctx, s.execFor(r), "vsftpd", 200)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"logs": text, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": text})
}

func (s *Server) ftpHomes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{"paths": ftp.Homes(ctx)})
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
	out, err := s.execFor(r).Run(ctx, "ftp-user-password", []string{body.Name}, body.Password)
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
	out, err := s.execFor(r).Run(ctx, "ftp-user-lock", []string{body.Name, mode}, "")
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
	out, err := s.execFor(r).Run(ctx, "ftp-user-write", []string{body.Name, mode}, "")
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
	out, err := s.execFor(r).Run(ctx, "ftp-user-add", []string{body.Name, body.Home}, body.Password)
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
	out, err := s.execFor(r).Run(ctx, "ftp-user-delete", []string{body.Name}, "")
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
