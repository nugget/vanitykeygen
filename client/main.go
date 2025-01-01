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

var (
	logger   *slog.Logger
	logLevel *slog.LevelVar
)

type hit struct {
	timestamp time.Time
	sid       int
	keyCount  int
}

type telemetry struct {
	startTime time.Time
	keyCount  int
	hitCount  int
}

func newTelemetry() telemetry {
	return telemetry{
		startTime: time.Now(),
		keyCount:  0,
		hitCount:  0,
	}
}

func (h hit) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Time("timestamp", h.timestamp),
		slog.Int("sid", h.sid),
	)
}

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
			keyCount:  r,
		}
		hits <- h
	}
}

func displayStats(t *telemetry) {
	wallTime := time.Now().Sub(t.startTime)

	hitRate := fmt.Sprintf("%0.02f", float64(t.hitCount)/float64(t.keyCount)*100)

	logger.Info("Runtime Stats",
		"runtime", wallTime,
		"keyCount", t.keyCount,
		"hitCount", t.hitCount,
		"hitRate", hitRate,
	)
}

func recordHit(h hit, runtimeStats *telemetry) error {
	runtimeStats.hitCount++
	runtimeStats.keyCount += h.keyCount

	logger.Warn("run select hit", "h", h)

	return nil
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	setupLogger(ctx, stdout)

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

	runtimeStats := newTelemetry()
	statsTrigger := time.Tick(10 * time.Second)

RunLoop:
	for {
		select {
		case <-statsTrigger:
			displayStats(&runtimeStats)
		case h := <-hits:
			err := recordHit(h, &runtimeStats)
			if err != nil {
				logger.Warn("unable to record hit",
					"hit", h,
					"error", err,
				)
			}
		case <-ctx.Done():
			stop()
			break RunLoop
		default:
			time.Sleep(250 * time.Millisecond)
		}
	}
	logger.Warn("interrupt detected", "err", ctx.Err())

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
