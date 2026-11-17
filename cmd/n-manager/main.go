package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"n-manager/internal/api"
	"n-manager/internal/auth"
	"n-manager/internal/config"
	"n-manager/internal/db"
	"n-manager/internal/monitor"
	"n-manager/internal/priv"
	"n-manager/internal/version"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "priv":
			os.Exit(priv.Main(os.Args[2:]))
		case "hash-password":
			os.Exit(hashPassword(os.Args[2:]))
		case "version":
			fmt.Println(version.Version)
			return
		case "serve":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		default:
			if !strings.HasPrefix(os.Args[1], "-") {
				usage()
				os.Exit(2)
			}
		}
	}
	if err := serve(); err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "N Manager %s\nusage: n-manager [serve] [--config path]\n       n-manager hash-password\n       n-manager priv <operation> [args]\n       n-manager version\n", version.Version)
}

func hashPassword(args []string) int {
	var pw []byte
	var err error
	switch len(args) {
	case 0:
		pw, err = io.ReadAll(os.Stdin)
		pw = []byte(strings.TrimRight(string(pw), "\r\n"))
	case 1:
		pw = []byte(args[0])
	default:
		fmt.Fprintln(os.Stderr, "usage: n-manager hash-password [password]")
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	hash, err := auth.HashPassword(string(pw))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println(hash)
	return 0
}

func serve() error {
	fs := flag.NewFlagSet("n-manager", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config.yaml")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	cfg, err := config.Load(config.ResolvePath(*configPath))
	if err != nil {
		return err
	}
	database, err := db.Open(cfg.Data.Database.Path)
	if err != nil {
		return err
	}
	defer database.Close()
	collector := monitor.NewCollector(time.Duration(cfg.Data.Monitoring.RefreshInterval)*time.Second, version.Version)
	collector.Start()
	defer collector.Stop()
	var exec priv.Executor = priv.Sudo{Command: cfg.Data.Privileged.Command}
	if cfg.Data.Security.DryRun {
		exec = priv.DryRun{}
		log.Printf("dry-run mode: privileged changes are validated and not applied")
	}
	srv := &http.Server{
		Addr:              cfg.Data.Addr(),
		Handler:           api.New(cfg, database, collector, exec).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		WriteTimeout:      15 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("N Manager %s listening on http://%s", version.Version, cfg.Data.Addr())
		var err error
		if cfg.Data.Server.TLSCert != "" && cfg.Data.Server.TLSKey != "" {
			err = srv.ListenAndServeTLS(cfg.Data.Server.TLSCert, cfg.Data.Server.TLSKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	case err := <-errCh:
		return err
	}
}
