package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrNoServer is returned when a management request has no selected server.
var ErrNoServer = errors.New("select a server")

// System runs commands and file operations on one machine.
type System interface {
	Run(ctx context.Context, name string, args []string, stdin string) (string, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error
	Stat(ctx context.Context, path string) (os.FileInfo, error)
	ReadDir(ctx context.Context, path string) ([]os.DirEntry, error)
	Glob(ctx context.Context, pattern string) ([]string, error)
	MkdirAll(ctx context.Context, path string, mode os.FileMode) error
	Remove(ctx context.Context, path string) error
	Symlink(ctx context.Context, target, link string) error
	Readlink(ctx context.Context, path string) (string, error)
	Rename(ctx context.Context, oldPath, newPath string) error
}

type key struct{}

// With attaches a machine to the context. A nil system keeps the local machine.
func With(ctx context.Context, sys System) context.Context {
	if sys == nil {
		return ctx
	}
	return context.WithValue(ctx, key{}, sys)
}

// From returns the machine stored on ctx, or the local computer.
func From(ctx context.Context) System {
	if ctx == nil {
		return Local{}
	}
	sys, _ := ctx.Value(key{}).(System)
	if sys == nil {
		return Local{}
	}
	return sys
}

func Run(ctx context.Context, name string, args []string, stdin string) (string, error) {
	return From(ctx).Run(ctx, name, args, stdin)
}

func ReadFile(ctx context.Context, path string) ([]byte, error) {
	return From(ctx).ReadFile(ctx, path)
}

func WriteFile(ctx context.Context, path string, data []byte, mode os.FileMode) error {
	return From(ctx).WriteFile(ctx, path, data, mode)
}

func Stat(ctx context.Context, path string) (os.FileInfo, error) {
	return From(ctx).Stat(ctx, path)
}

func ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	return From(ctx).ReadDir(ctx, path)
}

func Glob(ctx context.Context, pattern string) ([]string, error) {
	return From(ctx).Glob(ctx, pattern)
}

func MkdirAll(ctx context.Context, path string, mode os.FileMode) error {
	return From(ctx).MkdirAll(ctx, path, mode)
}

func Remove(ctx context.Context, path string) error {
	return From(ctx).Remove(ctx, path)
}

func Symlink(ctx context.Context, target, link string) error {
	return From(ctx).Symlink(ctx, target, link)
}

func Readlink(ctx context.Context, path string) (string, error) {
	return From(ctx).Readlink(ctx, path)
}

func Rename(ctx context.Context, oldPath, newPath string) error {
	return From(ctx).Rename(ctx, oldPath, newPath)
}

// Local is the computer running N Manager.
type Local struct{}

func (Local) Run(ctx context.Context, name string, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", err
		}
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
}

func (Local) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (Local) WriteFile(_ context.Context, path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}

func (Local) Stat(_ context.Context, path string) (os.FileInfo, error) {
	return os.Lstat(path)
}

func (Local) ReadDir(_ context.Context, path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

func (Local) Glob(_ context.Context, pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

func (Local) MkdirAll(_ context.Context, path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (Local) Remove(_ context.Context, path string) error {
	return os.Remove(path)
}

func (Local) Symlink(_ context.Context, target, link string) error {
	return os.Symlink(target, link)
}

func (Local) Readlink(_ context.Context, path string) (string, error) {
	return os.Readlink(path)
}

func (Local) Rename(_ context.Context, oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// Failure makes every operation return the same error.
type Failure struct{ Err error }

func (f Failure) Run(context.Context, string, []string, string) (string, error) {
	return "", f.Err
}
func (f Failure) ReadFile(context.Context, string) ([]byte, error) { return nil, f.Err }
func (f Failure) WriteFile(context.Context, string, []byte, os.FileMode) error {
	return f.Err
}
func (f Failure) Stat(context.Context, string) (os.FileInfo, error) { return nil, f.Err }
func (f Failure) ReadDir(context.Context, string) ([]os.DirEntry, error) {
	return nil, f.Err
}
func (f Failure) Glob(context.Context, string) ([]string, error) { return nil, f.Err }
func (f Failure) MkdirAll(context.Context, string, os.FileMode) error {
	return f.Err
}
func (f Failure) Remove(context.Context, string) error             { return f.Err }
func (f Failure) Symlink(context.Context, string, string) error    { return f.Err }
func (f Failure) Readlink(context.Context, string) (string, error) { return "", f.Err }
func (f Failure) Rename(context.Context, string, string) error     { return f.Err }

// Missing refuses management when the navbar has no server selected.
type Missing struct{}

func (Missing) Run(context.Context, string, []string, string) (string, error) {
	return "", ErrNoServer
}
func (Missing) ReadFile(context.Context, string) ([]byte, error) { return nil, ErrNoServer }
func (Missing) WriteFile(context.Context, string, []byte, os.FileMode) error {
	return ErrNoServer
}
func (Missing) Stat(context.Context, string) (os.FileInfo, error) { return nil, ErrNoServer }
func (Missing) ReadDir(context.Context, string) ([]os.DirEntry, error) {
	return nil, ErrNoServer
}
func (Missing) Glob(context.Context, string) ([]string, error) { return nil, ErrNoServer }
func (Missing) MkdirAll(context.Context, string, os.FileMode) error {
	return ErrNoServer
}
func (Missing) Remove(context.Context, string) error             { return ErrNoServer }
func (Missing) Symlink(context.Context, string, string) error    { return ErrNoServer }
func (Missing) Readlink(context.Context, string) (string, error) { return "", ErrNoServer }
func (Missing) Rename(context.Context, string, string) error     { return ErrNoServer }

// File is a directory entry returned by a remote listing.
type File struct {
	name string
	dir  bool
	mode os.FileMode
	size int64
}

func NewFile(name string, dir bool, mode os.FileMode) File {
	return File{name: name, dir: dir, mode: mode}
}

func (f File) Name() string               { return f.name }
func (f File) IsDir() bool                { return f.dir }
func (f File) Type() os.FileMode          { return f.mode.Type() }
func (f File) Info() (os.FileInfo, error) { return f, nil }
func (f File) Size() int64                { return f.size }
func (f File) Mode() os.FileMode          { return f.mode }
func (f File) ModTime() time.Time         { return time.Time{} }
func (f File) Sys() any                   { return nil }

func NewStat(name string, dir bool, mode os.FileMode, size int64) File {
	return File{name: name, dir: dir, mode: mode, size: size}
}

// ReadTail returns the last lines of a text file.
func ReadTail(ctx context.Context, path string, lines int) (string, error) {
	if lines < 1 {
		lines = 100
	}
	if lines > 500 {
		lines = 500
	}
	return Run(ctx, "tail", []string{"-n", fmt.Sprintf("%d", lines), "--", path}, "")
}

var (
	_ os.FileInfo = File{}
	_ os.DirEntry = File{}
)
