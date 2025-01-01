package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"
)

var (
	logger   *slog.Logger
	logLevel *slog.LevelVar
	target   string
)

func handleTarget(w http.ResponseWriter, req *http.Request) {
	target := `(?i)[\/\+](nugget|horse|slacker|wicca|wheelsdown|hollowoak|ferrari|porsche|gt3rs|portofino|longhorn|miata|equiraptor|equi|nugget)=?$`

	fmt.Fprintf(w, target)
	logger.Info("gave target", "target", target)
}

func handleHit(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, target)
	logger.Info("received hit")
}

func handlePing(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, target)
	logger.Info("received ping")
}

func setupLogger(ctx context.Context, stdout io.Writer) {
	logLevel = new(slog.LevelVar)

	handlerOptions := &slog.HandlerOptions{
		Level: logLevel,
	}
	handler := slog.NewTextHandler(stdout, handlerOptions)

	logger = slog.New(handler)
}

// run is the real main, but one where we can exit with an error.
func run(ctx context.Context, stdout io.Writer, stderr io.Writer, getenv func(string) string, args []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	setupLogger(ctx, stdout)

	myFlags := flag.NewFlagSet("myFlags", flag.ExitOnError)

	var _ = myFlags.Bool("v", false, "Verbose logging")

	err := myFlags.Parse(args[1:])
	if err != nil {
		return err
	}

	handlerOptions := &slog.HandlerOptions{Level: logLevel}
	logger = slog.New(slog.NewTextHandler(os.Stdout, handlerOptions))

	http.HandleFunc("/target", handleTarget)
	http.HandleFunc("/hit", handleHit)
	http.HandleFunc("/ping", handlePing)

	go func() {
		err = http.ListenAndServe(":8192", nil)
		if errors.Is(err, http.ErrServerClosed) {
			logger.Warn("server closed")
		} else if err != nil {
			logger.Error("server died", "error", err)
			os.Exit(1)
		}
	}()

RunLoop:
	for {
		select {
		case <-ctx.Done():
			stop()
			break RunLoop
		default:
			time.Sleep(250 * time.Millisecond)
		}
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
