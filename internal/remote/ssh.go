package remote

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// KeyChange means the server presented a different host key than the one we saved.
type KeyChange struct {
	Fingerprint string
	Key         []byte
}

func (e *KeyChange) Error() string {
	return "ssh host key changed"
}

// Endpoint is one SSH login.
type Endpoint struct {
	Host     string
	Port     int
	User     string
	Password string
	HostKey  []byte
}

// SSH is one open connection. Commands run as root, or through sudo -S.
type SSH struct {
	client   *ssh.Client
	user     string
	password string
	once     sync.Once
	mu       sync.Mutex
	tracer   Tracer
	label    string
	root     bool
	nopasswd bool
	probeErr error
}

// Dial opens an SSH connection. When HostKey is empty the presented key is returned for storage.
func Dial(ctx context.Context, ep Endpoint) (*SSH, []byte, error) {
	if ep.Port == 0 {
		ep.Port = 22
	}
	var presented []byte
	cfg := &ssh.ClientConfig{
		User:            ep.User,
		Auth:            []ssh.AuthMethod{ssh.Password(ep.Password), ssh.KeyboardInteractive(keyboard(ep.Password))},
		Timeout:         15 * time.Second,
		HostKeyCallback: hostKeyCallback(ep.HostKey, &presented),
	}
	addr := net.JoinHostPort(ep.Host, strconv.Itoa(ep.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh dial: %w", err)
	}
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		var change *KeyChange
		if errors.As(err, &change) {
			return nil, nil, change
		}
		return nil, nil, fmt.Errorf("ssh login failed")
	}
	label := ep.User + "@" + ep.Host
	return &SSH{client: ssh.NewClient(clientConn, chans, reqs), user: ep.User, password: ep.Password, label: label}, presented, nil
}

// SetTrace attaches the debug recorder. The label is the server name shown in the log.
func (s *SSH) SetTrace(t Tracer, label string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.tracer = t
	if label != "" {
		s.label = label
	}
	s.mu.Unlock()
}

func (s *SSH) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *SSH) Run(ctx context.Context, name string, args []string, stdin string) (string, error) {
	if err := s.prepare(ctx); err != nil {
		return "", err
	}
	if err := safeArg(name); err != nil {
		return "", err
	}
	for _, arg := range args {
		if err := safeArg(arg); err != nil {
			return "", err
		}
	}
	cmd := shellJoin(append([]string{name}, args...))
	payload := stdin
	if !s.root {
		if s.nopasswd {
			cmd = "sudo -n -- " + cmd
		} else {
			cmd = "sudo -S -p '' -- " + cmd
			payload = s.password + "\n" + stdin
		}
	}
	return s.exec(ctx, cmd, payload)
}

func (s *SSH) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := safePath(path); err != nil {
		return nil, err
	}
	out, err := s.Run(ctx, "base64", []string{"-w", "0", "--", path}, "")
	if err != nil {
		return nil, notFound(err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return []byte{}, nil
	}
	return base64.StdEncoding.DecodeString(out)
}

func (s *SSH) WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error {
	if err := safePath(path); err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	modeText := strconv.FormatUint(uint64(mode.Perm()), 8)
	script := "base64 -d | tee " + shellQuote(path) + " >/dev/null && chmod " + shellQuote(modeText) + " " + shellQuote(path)
	_, err := s.Run(ctx, "bash", []string{"-c", script}, encoded)
	return err
}

func (s *SSH) Stat(ctx context.Context, path string) (os.FileInfo, error) {
	if err := safePath(path); err != nil {
		return nil, err
	}
	out, err := s.Run(ctx, "stat", []string{"-c", "%F|%a|%s", "--", path}, "")
	if err != nil {
		return nil, notFound(err)
	}
	kind, rest, ok := strings.Cut(out, "|")
	if !ok {
		return nil, fmt.Errorf("stat failed")
	}
	modeText, sizeText, ok := strings.Cut(rest, "|")
	if !ok {
		return nil, fmt.Errorf("stat failed")
	}
	modeBits, err := strconv.ParseUint(strings.TrimSpace(modeText), 8, 32)
	if err != nil {
		return nil, err
	}
	size, err := strconv.ParseInt(strings.TrimSpace(sizeText), 10, 64)
	if err != nil {
		return nil, err
	}
	mode := os.FileMode(modeBits)
	dir := strings.Contains(kind, "directory")
	if strings.Contains(kind, "symbolic link") {
		mode |= os.ModeSymlink
	}
	if dir {
		mode |= os.ModeDir
	}
	return NewStat(path, dir, mode, size), nil
}

func (s *SSH) ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	if err := safePath(path); err != nil {
		return nil, err
	}
	out, err := s.Run(ctx, "find", []string{path, "-mindepth", "1", "-maxdepth", "1", "-printf", "%y\t%f\\n"}, "")
	if err != nil {
		return nil, notFound(err)
	}
	var entries []os.DirEntry
	for _, line := range strings.Split(out, "\n") {
		kind, name, ok := strings.Cut(line, "\t")
		if !ok || name == "" || name == "." || name == ".." {
			continue
		}
		mode := os.FileMode(0)
		if kind == "d" {
			mode = os.ModeDir
		}
		if kind == "l" {
			mode = os.ModeSymlink
		}
		entries = append(entries, NewFile(name, kind == "d", mode))
	}
	if entries == nil {
		entries = []os.DirEntry{}
	}
	return entries, nil
}

func (s *SSH) Glob(ctx context.Context, pattern string) ([]string, error) {
	if err := safeGlob(pattern); err != nil {
		return nil, err
	}
	root := pattern
	if i := strings.IndexAny(pattern, "*?["); i >= 0 {
		root = pattern[:i]
		if j := strings.LastIndex(root, "/"); j > 0 {
			root = root[:j]
		}
	}
	out, err := s.Run(ctx, "find", []string{root, "-path", pattern}, "")
	if err != nil && out == "" {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "*") {
			continue
		}
		paths = append(paths, line)
	}
	return paths, nil
}

func (s *SSH) MkdirAll(ctx context.Context, path string, mode os.FileMode) error {
	if err := safePath(path); err != nil {
		return err
	}
	_, err := s.Run(ctx, "mkdir", []string{"-p", path}, "")
	return err
}

func (s *SSH) Remove(ctx context.Context, path string) error {
	if err := safePath(path); err != nil {
		return err
	}
	_, err := s.Run(ctx, "rm", []string{"-f", "--", path}, "")
	return err
}

func (s *SSH) Symlink(ctx context.Context, target, link string) error {
	if err := safePath(link); err != nil {
		return err
	}
	if strings.ContainsAny(target, "\n\r\x00") {
		return fmt.Errorf("invalid path")
	}
	_, err := s.Run(ctx, "ln", []string{"-sfn", "--", target, link}, "")
	return err
}

func (s *SSH) Readlink(ctx context.Context, path string) (string, error) {
	if err := safePath(path); err != nil {
		return "", err
	}
	out, err := s.Run(ctx, "readlink", []string{"-n", "--", path}, "")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "permission") {
			return "", err
		}
		return "", &os.PathError{Op: "readlink", Path: path, Err: os.ErrNotExist}
	}
	return out, nil
}

func (s *SSH) Rename(ctx context.Context, oldPath, newPath string) error {
	if err := safePath(oldPath); err != nil {
		return err
	}
	if err := safePath(newPath); err != nil {
		return err
	}
	_, err := s.Run(ctx, "mv", []string{"-f", "--", oldPath, newPath}, "")
	return err
}

func notFound(err error) error {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no such file") || strings.Contains(msg, "does not exist") || strings.Contains(msg, "not a directory") {
		return &os.PathError{Op: "open", Err: os.ErrNotExist}
	}
	return err
}

func (s *SSH) prepare(ctx context.Context) error {
	s.once.Do(func() {
		out, err := s.exec(ctx, "id -u", "")
		if err != nil {
			s.probeErr = err
			return
		}
		if strings.TrimSpace(out) == "0" || s.user == "root" {
			s.root = true
			return
		}
		if _, err := s.exec(ctx, "sudo -n true", ""); err == nil {
			s.nopasswd = true
		}
	})
	return s.probeErr
}

func (s *SSH) exec(ctx context.Context, cmd, stdin string) (string, error) {
	session, err := s.client.NewSession()
	if err != nil {
		s.traceCmd(ctx, cmd, err.Error())
		return "", err
	}
	defer session.Close()
	session.Stdin = stringsReader(stdin)
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()
	select {
	case <-ctx.Done():
		_ = session.Close()
		if ctx.Err() == context.DeadlineExceeded {
			s.traceCmd(ctx, cmd, "timed out")
		}
		return "", ctx.Err()
	case err := <-done:
		text := strings.TrimSpace(stdout.String())
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = text
			}
			msg = s.redact(msg)
			if msg == "" {
				msg = err.Error()
			}
			s.traceCmd(ctx, cmd, msg)
			if strings.Contains(msg, "No such file") || strings.Contains(msg, "Not a directory") {
				return "", fmt.Errorf("%s: %w", msg, os.ErrNotExist)
			}
			return msg, fmt.Errorf("%s", msg)
		}
		return text, nil
	}
}

func (s *SSH) traceCmd(ctx context.Context, cmd, detail string) {
	if ctx.Err() == context.Canceled || skipTrace(cmd) {
		return
	}
	s.mu.Lock()
	t := s.tracer
	label := s.label
	s.mu.Unlock()
	if t == nil {
		return
	}
	t.Note("error", label, "command", s.redact(cmd), s.redact(detail))
}

func skipTrace(cmd string) bool {
	switch strings.TrimSpace(cmd) {
	case "true", "id -u", "sudo -n true", "sudo -n -- true", "sudo -n -- id -u", "sudo -S -p '' -- true", "sudo -S -p '' -- id -u":
		return true
	default:
		return false
	}
}

func (s *SSH) redact(text string) string {
	if s.password == "" || text == "" {
		return text
	}
	return strings.ReplaceAll(text, s.password, "")
}

func hostKeyCallback(stored []byte, presented *[]byte) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		raw := key.Marshal()
		*presented = append([]byte(nil), raw...)
		if len(stored) == 0 {
			return nil
		}
		if string(stored) == string(raw) {
			return nil
		}
		return &KeyChange{Fingerprint: ssh.FingerprintSHA256(key), Key: append([]byte(nil), raw...)}
	}
}

func keyboard(password string) ssh.KeyboardInteractiveChallenge {
	return func(name, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range questions {
			answers[i] = password
		}
		return answers, nil
	}
}

func shellJoin(args []string) string {
	var b strings.Builder
	for i, arg := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(shellQuote(arg))
	}
	return b.String()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func safeArg(value string) error {
	if strings.ContainsAny(value, "\n\r\x00") {
		return fmt.Errorf("invalid command argument")
	}
	return nil
}

func safePath(path string) error {
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "..") || strings.ContainsAny(path, "\n\r\x00") {
		return fmt.Errorf("invalid path")
	}
	return nil
}

func safeGlob(pattern string) error {
	if pattern == "" || !strings.HasPrefix(pattern, "/") || strings.ContainsAny(pattern, "\n\r\x00;|&$`<>(){}[]!#") {
		return fmt.Errorf("invalid path")
	}
	return nil
}

func stringsReader(value string) *bytes.Reader {
	return bytes.NewReader([]byte(value))
}
