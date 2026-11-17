package priv

import (
	"context"
	"strings"

	"n-manager/internal/remote"
	"n-manager/internal/validate"
)

type Executor interface {
	Run(ctx context.Context, op string, args []string, stdin string) (string, error)
}

type DryRun struct{}

func (DryRun) Run(_ context.Context, op string, args []string, stdin string) (string, error) {
	if err := validate.Operation(op, args); err != nil {
		return "", err
	}
	if op == "nginx-apply" {
		if _, err := validate.NginxApply(stdin); err != nil {
			return "", err
		}
	}
	if err := validateStdin(op, stdin); err != nil {
		return "", err
	}
	return "dry-run: " + op + " " + strings.Join(args, " "), nil
}

type Sudo struct {
	Command string
}

func (s Sudo) Run(ctx context.Context, op string, args []string, stdin string) (string, error) {
	if err := validate.Operation(op, args); err != nil {
		return "", err
	}
	if op == "nginx-apply" {
		if _, err := validate.NginxApply(stdin); err != nil {
			return "", err
		}
	}
	if err := validateStdin(op, stdin); err != nil {
		return "", err
	}
	binary := s.Command
	if binary == "" {
		binary = "/usr/bin/n-manager"
	}
	argv := append([]string{"-n", binary, "priv", op}, args...)
	return run(ctx, "sudo", argv, stdin)
}

func validateStdin(op, stdin string) error {
	switch op {
	case "ftp-apply":
		_, err := validate.FTPApplyPayload(stdin)
		return err
	case "ftp-user-add", "ftp-user-password", "mysql-user-create", "mysql-user-password":
		return validate.FTPPassword(stdin)
	case "mysql-db-import":
		return validate.MySQLDump(stdin)
	case "nginx-cert-put":
		return validate.NginxCertPayload(stdin)
	default:
		return nil
	}
}

func run(ctx context.Context, name string, args []string, stdin string) (string, error) {
	return remote.Run(ctx, name, args, stdin)
}
