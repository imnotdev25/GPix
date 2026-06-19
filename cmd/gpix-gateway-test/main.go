// Command gpix-gateway-test runs the S3 and WebDAV gateways against an
// in-memory backend (no Google Photos credentials required). It exists so the
// gateway protocol layers can be exercised end-to-end with real S3/WebDAV
// clients (boto3, aws-cli, mc, rclone, curl).
//
//	go run ./cmd/gpix-gateway-test \
//	    -s3 127.0.0.1:9000 -dav 127.0.0.1:8081 \
//	    -access test -secret testsecret -bucket gpix \
//	    -user gpix -pass gpix
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"flag"

	"gpix/pkg/dav"
	"gpix/pkg/s3"
	"gpix/pkg/store"
)

func main() {
	s3Addr := flag.String("s3", "127.0.0.1:9000", "S3 listen address ('' to disable)")
	davAddr := flag.String("dav", "127.0.0.1:8081", "WebDAV listen address ('' to disable)")
	access := flag.String("access", "test", "S3 access key id")
	secret := flag.String("secret", "testsecret", "S3 secret access key")
	bucket := flag.String("bucket", "gpix", "S3 bucket name")
	user := flag.String("user", "gpix", "WebDAV basic-auth username")
	pass := flag.String("pass", "gpix", "WebDAV basic-auth password")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	be := store.NewMemBackend()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var runners []func(context.Context) error

	if *s3Addr != "" {
		srv, err := s3.New(s3.Config{
			Listen:    *s3Addr,
			AccessKey: *access,
			SecretKey: *secret,
			Bucket:    *bucket,
		}, be, log.With("svc", "s3"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "s3:", err)
			os.Exit(1)
		}
		runners = append(runners, srv.Run)
	}

	if *davAddr != "" {
		authFn := func(u, p string) bool { return u == *user && p == *pass }
		srv, err := dav.New(dav.Config{Listen: *davAddr, BasePath: "/"}, be, authFn, log.With("svc", "dav"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "dav:", err)
			os.Exit(1)
		}
		runners = append(runners, srv.Run)
	}

	errCh := make(chan error, len(runners))
	for _, run := range runners {
		run := run
		go func() { errCh <- run(ctx) }()
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			fmt.Fprintln(os.Stderr, "server error:", err)
			os.Exit(1)
		}
	}
}
