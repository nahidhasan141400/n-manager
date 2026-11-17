package api

import (
	"net/http"
	"strconv"

	"n-manager/internal/firewall"
	"n-manager/internal/pkgmgr"
)

func (s *Server) firewallStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	status, err := firewall.Collect(ctx)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	status.SSHPort = s.sshPort(r)
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) firewallInstall(w http.ResponseWriter, r *http.Request) {
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
	if mgr.ID != "apt" {
		writeErr(w, http.StatusBadRequest, "this firewall manager supports UFW on Debian and Ubuntu")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, "pkg-install", []string{mgr.ID, "ufw"}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "firewall.install", "ufw", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) firewallEnable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &body); err != nil || !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, "firewall-enable", []string{strconv.Itoa(s.sshPort(r))}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "firewall.enable", "ufw", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) firewallDisable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &body); err != nil || !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, "firewall-disable", nil, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "firewall.disable", "ufw", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) firewallAllow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action    string `json:"action"`
		Direction string `json:"direction"`
		Port      int    `json:"port"`
		Protocol  string `json:"protocol"`
		From      string `json:"from"`
		Comment   string `json:"comment"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Action == "" {
		body.Action = "allow"
	}
	if body.Direction == "" {
		body.Direction = "in"
	}
	if body.Protocol == "" {
		body.Protocol = "tcp"
	}
	if body.From == "" {
		body.From = "any"
	}
	comment := body.Comment
	if comment == "" {
		comment = "-"
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, "firewall-allow", []string{body.Action, body.Direction, strconv.Itoa(body.Port), body.Protocol, body.From, comment, strconv.Itoa(s.sshPort(r))}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "firewall.allow", body.Action+" "+strconv.Itoa(body.Port)+"/"+body.Protocol, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) firewallUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Number    int    `json:"number"`
		Action    string `json:"action"`
		Direction string `json:"direction"`
		Port      int    `json:"port"`
		Protocol  string `json:"protocol"`
		From      string `json:"from"`
		Comment   string `json:"comment"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Action == "" {
		body.Action = "allow"
	}
	if body.Direction == "" {
		body.Direction = "in"
	}
	if body.Protocol == "" {
		body.Protocol = "tcp"
	}
	if body.From == "" {
		body.From = "any"
	}
	comment := body.Comment
	if comment == "" {
		comment = "-"
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, "firewall-update", []string{strconv.Itoa(body.Number), body.Action, body.Direction, strconv.Itoa(body.Port), body.Protocol, body.From, comment, strconv.Itoa(s.sshPort(r))}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "firewall.update", strconv.Itoa(body.Number), dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) firewallConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Incoming string `json:"incoming"`
		Outgoing string `json:"outgoing"`
		Logging  string `json:"logging"`
		IPv6     bool   `json:"ipv6"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ipv6 := "no"
	if body.IPv6 {
		ipv6 = "yes"
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, "firewall-config", []string{body.Incoming, body.Outgoing, body.Logging, ipv6, strconv.Itoa(s.sshPort(r))}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "firewall.config", body.Incoming+"/"+body.Outgoing, dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) firewallDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Number  int  `json:"number"`
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation is required")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := s.execFor(r).Run(ctx, "firewall-delete", []string{strconv.Itoa(body.Number), strconv.Itoa(s.sshPort(r))}, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "firewall.delete", strconv.Itoa(body.Number), dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) sshPort(r *http.Request) int {
	id := r.Header.Get("X-Server-Id")
	rec, err := s.servers.Get(id)
	if err != nil || rec.Port < 1 || rec.Port > 65535 {
		return 22
	}
	return rec.Port
}
