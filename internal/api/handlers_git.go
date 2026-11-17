package api

import (
	"net/http"
	"strings"

	"n-manager/internal/apps"
)

func (s *Server) gitStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := s.withMachine(w, r)
	if !ok {
		return
	}
	defer cancel()
	writeJSON(w, http.StatusOK, apps.Git(ctx))
}

func (s *Server) gitInstall(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.InstallGit(ctx, s.execFor(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "git.install", "git", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) gitConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.ConfigureGit(ctx, s.execFor(r), body.Name, body.Email)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "git.config", strings.TrimSpace(body.Email), dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) gitClone(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL  string `json:"url"`
		Path string `json:"path"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.CloneGit(ctx, s.execFor(r), body.URL, body.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "git.clone", strings.TrimSpace(body.Path), dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) gitPull(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path    string `json:"path"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !body.Confirm {
		writeErr(w, http.StatusBadRequest, "confirmation required")
		return
	}
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.PullGit(ctx, s.execFor(r), body.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "git.pull", strings.TrimSpace(body.Path), dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, result(s.dry(), out))
}

func (s *Server) gitGitHub(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.ConnectGitHub(ctx, s.execFor(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "git.github", "github", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, gitResult(s.dry(), out))
}

func (s *Server) gitGitHubTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.opCtx(r)
	defer cancel()
	out, err := apps.TestGitHub(ctx, s.execFor(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.record(r, "git.github.test", "github", dryDetail(s.dry()))
	writeJSON(w, http.StatusOK, gitResult(s.dry(), out))
}

func gitResult(dry bool, output string) map[string]any {
	res := result(dry, output)
	if !dry && output != "" {
		res["message"] = output
	}
	return res
}
