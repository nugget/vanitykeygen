package server

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

	listenPort    int
	listenAddress string
	matchLogFile  string
)

func FlagSet() *flag.FlagSet {
	f := flag.NewFlagSet("server", flag.ExitOnError)

	f.IntVar(&listenPort, "p", 8080, "Specifies the port on which the server listens for connections")
	f.StringVar(&listenAddress, "b", "", "Bind this address on the local machine when listening for connections (default '' for all addresses)")
	f.StringVar(&matchLogFile, "l", "matchfile.log", "Log successful matches to this file")

	return f
}

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
	MatchString          string    `json:"matchString"`
	MatchedAuthorizedKey bool      `json:"matchedAuthorizedKey"`
	MatchedFingerprint   bool      `json:"matchedFingerprint"`
	Key                  Key       `json:"key"`
}

func handleTarget(w http.ResponseWriter, req *http.Request) {
	target = os.Getenv("VKG_TARGET")
	if target == "" {
		target = `(?i)[\/\+](nugget|slacker|wheelsdown|hollowoak|ferrari|porsche|gt3rs|portofino|longhorn|miata|equiraptor|nugget-info|vanitykey|vanity-nugget)=?$`
	}
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

// run is the real main, but one where we can exit with an error.
func Run(ctx context.Context, l *slog.Logger, stdout io.Writer, stderr io.Writer, getenv func(string) string, args []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	logger = l

	logger.Info("Starting Server")

	myFlags := FlagSet()
	err := myFlags.Parse(args)
	if err != nil {
		return err
	}

	matchFile, err := os.OpenFile(matchLogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer matchFile.Close()
	matchLogger = slog.New(slog.NewJSONHandler(matchFile, nil))
	logger.Info("Logging matches to file", "matchLogFile", matchLogFile)

	http.HandleFunc("/target", handleTarget)
	http.HandleFunc("/match", handleMatch)

	go func() {
		addr := fmt.Sprintf("%s:%d", listenAddress, listenPort)
		logger.Info("listening", "addr", addr)

		err = http.ListenAndServe(addr, nil)
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
