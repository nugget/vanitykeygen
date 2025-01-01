package main

import (
	"context"
	"encoding/json"
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
	logger      *slog.Logger
	logLevel    *slog.LevelVar
	target      string
	matchLogger *slog.Logger
)

type Key struct {
	PrivateKey       []byte `json:"privateKey"`
	PublicKey        []byte `json:"publicKey"`
	EncodedKey       []byte `json:"encodedKey"`
	PrivateString    string `json:"privateString"`
	AuthorizedString string `json:"authorizedString"`
	Fingerprint      string `json:"fingerprint"`
}

type Match struct {
	Timestamp            time.Time `json:"timestamp"`
	Hostname             string    `json:"hostname"`
	SeekerID             int       `json:"seekerID"`
	MatchedAuthorizedKey bool      `json:"matchedAuthorizedKey"`
	MatchedFingerprint   bool      `json:"matchedFingerprint"`
	Key                  Key       `json:"key"`
}

func handleTarget(w http.ResponseWriter, req *http.Request) {
	target := `(?i)[\/\+](nugget|horse|slacker|wicca|wheelsdown|hollowoak|ferrari|porsche|gt3rs|portofino|longhorn|miata|equiraptor|equi|nugget)=?$`

	fmt.Fprintf(w, target)
	logger.Debug("gave target", "target", target)
}

func handleMatch(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "OK")
	logger.Debug("received match request", "request", req)

	decoder := json.NewDecoder(req.Body)
	var p Match
	err := decoder.Decode(&p)
	if err != nil {
		logger.Error("Decoder Failed", "error", err)
	}

	logger.Info("received match",
		"hostname", p.Hostname,
		"seekerID", p.SeekerID,
		"authKey", p.Key.AuthorizedString,
		"finger", p.Key.Fingerprint,
	)

	matchLogger.Info("match reported", "payload", p)
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

	matchFile, err := os.OpenFile("matchfile.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer matchFile.Close()
	matchLogger = slog.New(slog.NewJSONHandler(matchFile, nil))

	http.HandleFunc("/target", handleTarget)
	http.HandleFunc("/match", handleMatch)

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
