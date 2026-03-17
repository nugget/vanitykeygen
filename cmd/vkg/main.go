package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"

	"github.com/nugget/vanitykeygen/pkg/client"
	pkgserver "github.com/nugget/vanitykeygen/pkg/server"
)

var logger *slog.Logger

func setupLogger(stdout io.Writer, verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: level})
	logger = slog.New(handler)
}

func usage() {
	fmt.Println("usage: vkg <command> [options]")
	fmt.Println()
	fmt.Println("commands:")
	fmt.Println("  server    Start the VKG server")
	fmt.Println("  client    Start a VKG client")
	fmt.Println("  version   Show version info")
	fmt.Println()
	fmt.Println("server options:")
	pkgserver.FlagSet().PrintDefaults()
	fmt.Println()
	fmt.Println("client options:")
	client.FlagSet().PrintDefaults()
	os.Exit(0)
}

var gitVersion = "dev"

func run(ctx context.Context, stdout io.Writer, stderr io.Writer, getenv func(string) string, args []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	// Parse global flags
	verbose := false
	globalFlags := flag.NewFlagSet("vkg", flag.ContinueOnError)
	globalFlags.BoolVar(&verbose, "v", false, "Verbose (debug) logging")
	globalFlags.Parse(args[1:])

	setupLogger(stdout, verbose)

	remaining := globalFlags.Args()
	if len(remaining) == 0 {
		usage()
	}

	cmd := remaining[0]
	cmdArgs := remaining[1:]

	pkgserver.Version = gitVersion

	switch cmd {
	case "server":
		return pkgserver.Run(ctx, logger, stdout, stderr, getenv, cmdArgs)
	case "client":
		return client.Run(ctx, logger, stdout, stderr, getenv, cmdArgs)
	case "version":
		fmt.Fprintf(stdout, "vkg %s\n", gitVersion)
		return nil
	default:
		usage()
	}

	return nil
}

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Stdout, os.Stderr, os.Getenv, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
