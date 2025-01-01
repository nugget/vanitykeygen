package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"github.com/mikesmitty/edkey"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

var (
	logger   *slog.Logger
	logLevel *slog.LevelVar
	target   string
)

type seekerStatus struct {
	timestamp            time.Time
	sid                  int
	keyCount             int
	matchedAuthorizedKey bool
	matchedFingerprint   bool
	key                  GenerateKeyResult
}

type telemetry struct {
	launchStartTime time.Time
	searchStartTime time.Time
	keyCount        int
	hitCount        int
}

func newTelemetry() telemetry {
	return telemetry{
		launchStartTime: time.Now(),
		searchStartTime: time.Now(),
		keyCount:        1,
		hitCount:        0,
	}
}

func (s seekerStatus) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Time("timestamp", s.timestamp),
		slog.Int("sid", s.sid),
		slog.Int("keyCount", s.keyCount),
		slog.Bool("matchedAuthorizedKey", s.matchedAuthorizedKey),
		slog.Bool("matchedFingerprint", s.matchedFingerprint),
		slog.String("fingerprint", s.key.fingerprint),
		slog.String("auth", s.key.authorizedKey),
	)
}

type GenerateKeyResult struct {
	publicKey     ed25519.PublicKey
	privateKey    ed25519.PrivateKey
	sshKey        ssh.PublicKey
	pemKey        *pem.Block
	authorizedKey string
	fingerprint   string
	encodedKey    []byte
}

func GenerateKey(w io.Reader) (GenerateKeyResult, error) {
	var (
		k   GenerateKeyResult
		err error
	)

	k.publicKey, k.privateKey, err = ed25519.GenerateKey(w)
	if err != nil {
		return GenerateKeyResult{}, err
	}

	k.sshKey, err = ssh.NewPublicKey(k.publicKey)
	if err != nil {
		return GenerateKeyResult{}, err
	}

	k.pemKey = &pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: edkey.MarshalED25519PrivateKey(k.privateKey),
	}

	k.encodedKey = pem.EncodeToMemory(k.pemKey)

	k.authorizedKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k.sshKey)))

	h := sha256.New()
	h.Write(k.sshKey.Marshal())
	k.fingerprint = base64.StdEncoding.EncodeToString(h.Sum(nil))

	return k, nil
}

func seeker(ctx context.Context, statusUpdates chan seekerStatus, sid int) {
	logger := logger.With("sid", sid)
	logger.Info("seeker starting")

	var (
		lastTarget string
		re         *regexp.Regexp
		err        error
	)

	statusTrigger := time.Tick(5 * time.Second)

	keyCount := 0

	for {
		if target != lastTarget {
			re, err = regexp.Compile(target)
			if err != nil {
				logger.Error("unable to compile regexp", "error", err)
			}
			logger.Warn("new target detected", "lastTarget", lastTarget, "target", target, "re", re)
			lastTarget = target
		}

		if target == "" {
			logger.Info("no current target, sleeping")
			time.Sleep(1 * time.Minute)
		} else {

			k, err := GenerateKey(nil)
			if err != nil {
				logger.Warn("error generating key", "error", err)
				time.Sleep(1 * time.Minute)
			}

			matchedFingerprint := re.MatchString(k.fingerprint)
			matchedAuthorizedKey := re.MatchString(k.authorizedKey)

			if matchedFingerprint || matchedAuthorizedKey {
				s := seekerStatus{
					timestamp:            time.Now(),
					sid:                  sid,
					keyCount:             keyCount,
					matchedAuthorizedKey: matchedAuthorizedKey,
					matchedFingerprint:   matchedFingerprint,
					key:                  k,
				}
				statusUpdates <- s

				keyCount = 0
			}
		}

		select {
		case <-statusTrigger:
			s := seekerStatus{
				timestamp: time.Now(),
				sid:       sid,
				keyCount:  keyCount,
				key:       GenerateKeyResult{},
			}
			statusUpdates <- s
			keyCount = 0
		default:
			keyCount++
		}
	}
}

func displayStats(t *telemetry) {
	launchDuration := time.Now().Sub(t.launchStartTime)
	searchDuration := time.Now().Sub(t.searchStartTime)

	hitRate := float64(t.hitCount) / float64(t.keyCount) * 100

	logger.Info("Runtime Stats",
		"launchDuration", launchDuration,
		"searchDuration", searchDuration,
		"keyCount", t.keyCount,
		"hitCount", t.hitCount,
		"hitRate", hitRate,
	)
}

func recordStatus(s seekerStatus, t *telemetry) error {
	t.keyCount += s.keyCount

	if s.key.fingerprint != "" {
		logger.Warn("run select hit", "s", s)
		t.hitCount++
	}

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

func getTarget() (string, error) {
	r, err := http.Get("http://localhost:8192/target")
	if err != nil {
		return "", err
	}
	logger.Info("target requested", "code", r.StatusCode)

	resBody, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	logger.Info("target is", "target", resBody)

	target := string(resBody)
	target = `(?i)[\/\+](nugget|horse|ferrari|porsche|gt3rs|portofino|longhorn|miata|equiraptor|equi|nugget)=?$`

	return target, nil
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

	target, err = getTarget()
	if err != nil {
		return err
	}

	statusUpdates := make(chan seekerStatus)

	go seeker(ctx, statusUpdates, 1)
	go seeker(ctx, statusUpdates, 2)
	go seeker(ctx, statusUpdates, 3)

	runtimeStats := newTelemetry()
	statsTrigger := time.Tick(5 * time.Second)
	checkTarget := time.Tick(20 * time.Second)

RunLoop:
	for {
		select {
		case <-checkTarget:
			target, err = getTarget()
			if err != nil {
				target = ""
				logger.Error("Unable to fetch target", "error", err)
			}
			// runtimeStats.searchStartTime = time.Now()
			// runtimeStats.keyCount = 1
			// runtimeStats.hitCount = 0
		case <-statsTrigger:
			displayStats(&runtimeStats)
		case s := <-statusUpdates:
			err := recordStatus(s, &runtimeStats)
			if err != nil {
				logger.Warn("unable to record status",
					"hit", s,
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
