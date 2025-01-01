package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"time"
)

type hit struct {
	timestamp time.Time
	sid       int
}

func (h hit) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Time("timestamp", h.timestamp),
		slog.Int("sid", h.sid),
	)
}

var (
	logger   *slog.Logger
	logLevel *slog.LevelVar
)

func seeker(ctx context.Context, hits chan hit, sid int) {
	logger := logger.With("sid", sid)

	logger.Info("seeker starting")

	for {
		r := rand.Intn(10)
		logger.Info("seeker sleeping", "r", r)
		time.Sleep(time.Duration(r) * time.Second)

		h := hit{
			timestamp: time.Now(),
			sid:       sid,
		}
		hits <- h
	}
}

var neverReady = make(chan struct{}) // never closed

func recordHit(h hit) error {
	logger.Info("run select hit", "h", h)

	return nil
}

// run is the real main, but one where we can exit with an error.
func run(ctx context.Context, stdout io.Writer, stderr io.Writer, getenv func(string) string, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logLevel = new(slog.LevelVar)
	logLevel.Set(slog.LevelInfo)
	handlerOptions := &slog.HandlerOptions{
		Level: logLevel,
	}
	logger = slog.New(slog.NewTextHandler(os.Stdout, handlerOptions))

	myFlags := flag.NewFlagSet("myFlags", flag.ExitOnError)

	var _ = myFlags.Bool("v", false, "Verbose logging")

	err := myFlags.Parse(args[1:])
	if err != nil {
		return err
	}

	hits := make(chan hit)

	go seeker(ctx, hits, 1)
	go seeker(ctx, hits, 2)
	go seeker(ctx, hits, 3)

	logger.Info("run begin for")

RunLoop:
	for {
		select {
		case h := <-hits:
			err := recordHit(h)
			if err != nil {
				logger.Warn("unable to record hit",
					"hit", h,
					"error", err,
				)
			}
		case <-ctx.Done():
			logger.Warn("interrupt detected",
				"err", ctx.Err(),
			)
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
