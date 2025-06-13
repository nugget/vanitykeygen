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
	"github.com/nugget/vanitykeygen/pkg/server"
)

var (
	logger   *slog.Logger
	logLevel *slog.LevelVar
)

func setupLogger(ctx context.Context, stdout io.Writer) {
	logLevel = new(slog.LevelVar)

	handlerOptions := &slog.HandlerOptions{
		Level: logLevel,
	}
	handler := slog.NewTextHandler(stdout, handlerOptions)
	logger = slog.New(handler)
}

func FlagSet() *flag.FlagSet {
	f := flag.NewFlagSet("client", flag.ExitOnError)

	return f
}

func usage(f *flag.FlagSet) {
	fmt.Println("usage: <command> [<args>]")
	fmt.Println("")
	fmt.Println("global options")
	f.PrintDefaults()
	fmt.Println("client options")
	client.FlagSet().PrintDefaults()

	os.Exit(0)
}

// run is the real main, but one where we can exit with an error.
func run(ctx context.Context, stdout io.Writer, stderr io.Writer, getenv func(string) string, args []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	setupLogger(ctx, stdout)

	globalFlagSet := FlagSet()

	err := globalFlagSet.Parse(args[1:])
	if err != nil {
		return err
	}

	if len(args) < 2 {
		usage(globalFlagSet)
	}

	logger.Debug("Launching vkg", "args", args)

	switch args[1] {
	case "server":
		return server.Run(ctx, logger, os.Stdout, os.Stderr, os.Getenv, os.Args[2:])
	case "client":
		return client.Run(ctx, logger, os.Stdout, os.Stderr, os.Getenv, os.Args[2:])
	default:
		usage(globalFlagSet)
	}

	return nil
}

// main does as little as we can get away with.
func main() {
	ctx := context.Background()

	if err := run(ctx, os.Stdout, os.Stderr, os.Getenv, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
