package client

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nugget/vanitykeygen/pkg/keygen"
	"github.com/nugget/vanitykeygen/pkg/vkg"
)

var (
	serverURI  string
	numSeekers int
)

func FlagSet() *flag.FlagSet {
	f := flag.NewFlagSet("client", flag.ExitOnError)
	f.StringVar(&serverURI, "s", "https://vkg", "VKG server URI")
	f.IntVar(&numSeekers, "n", runtime.NumCPU(), "Number of seeker goroutines")
	return f
}

// Client is the VKG key-searching client.
type Client struct {
	logger     *slog.Logger
	httpClient *http.Client
	serverURI  string
	clientID   string
	hostname   string
	version    string
	seekers    int

	target   atomic.Value // stores *vkg.Target (or nil)
	keyCount atomic.Int64
}

type seekerStatus struct {
	timestamp            time.Time
	sid                  int
	keyCount             int
	matchString          string
	matchedAuthorizedKey bool
	matchedFingerprint   bool
	key                  keygen.Result
}

func (s seekerStatus) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Time("timestamp", s.timestamp),
		slog.Int("sid", s.sid),
		slog.Int("keyCount", s.keyCount),
		slog.Bool("matchedAuthorizedKey", s.matchedAuthorizedKey),
		slog.Bool("matchedFingerprint", s.matchedFingerprint),
		slog.String("fingerprint", s.key.Fingerprint),
		slog.String("auth", s.key.AuthorizedKey),
	)
}

type telemetry struct {
	launchStartTime time.Time
	keyCount        int
	hitCount        int
}

func (c *Client) currentTarget() *vkg.Target {
	v := c.target.Load()
	if v == nil {
		return nil
	}
	return v.(*vkg.Target)
}

func (c *Client) seeker(ctx context.Context, statusUpdates chan<- seekerStatus, sid int) {
	logger := c.logger.With("sid", sid)
	logger.Info("seeker starting")

	var (
		lastPattern string
		re          *regexp.Regexp
	)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	keyCount := 0

	for {
		select {
		case <-ctx.Done():
			logger.Info("seeker stopping")
			return
		default:
		}

		t := c.currentTarget()
		if t == nil || t.Pattern == "" {
			time.Sleep(1 * time.Second)
			continue
		}

		if t.Pattern != lastPattern {
			var err error
			re, err = regexp.Compile(t.Pattern)
			if err != nil {
				logger.Error("unable to compile regexp", "pattern", t.Pattern, "error", err)
				re = nil
				lastPattern = t.Pattern
				time.Sleep(5 * time.Second)
				continue
			}
			logger.Info("new target pattern", "pattern", t.Pattern)
			lastPattern = t.Pattern
		}

		if re == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		k, err := keygen.Generate()
		if err != nil {
			logger.Warn("error generating key", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}

		c.keyCount.Add(1)

		matchedFingerprint := re.MatchString(k.Fingerprint)
		matchedAuthorizedKey := re.MatchString(k.AuthorizedKey)

		if matchedFingerprint || matchedAuthorizedKey {
			var matchString strings.Builder
			matchString.WriteString(re.FindString(k.Fingerprint))
			matchString.WriteString(re.FindString(k.AuthorizedKey))

			s := seekerStatus{
				timestamp:            time.Now(),
				sid:                  sid,
				matchString:          matchString.String(),
				keyCount:             keyCount,
				matchedAuthorizedKey: matchedAuthorizedKey,
				matchedFingerprint:   matchedFingerprint,
				key:                  k,
			}
			select {
			case statusUpdates <- s:
			case <-ctx.Done():
				return
			}
			keyCount = 0
		}

		select {
		case <-ticker.C:
			s := seekerStatus{
				timestamp: time.Now(),
				sid:       sid,
				keyCount:  keyCount,
			}
			select {
			case statusUpdates <- s:
			case <-ctx.Done():
				return
			}
			keyCount = 0
		default:
			keyCount++
		}
	}
}

// fetchTargetFull fetches and stores the active target from the server.
func (c *Client) fetchTargetFull() error {
	resp, err := c.httpClient.Get(c.serverURI + "/api/targets/active")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Data *vkg.Target `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode target: %w", err)
	}

	c.target.Store(result.Data)
	if result.Data != nil {
		c.logger.Debug("target fetched", "pattern", result.Data.Pattern)
	} else {
		c.logger.Debug("no active target")
	}
	return nil
}

func (c *Client) register() error {
	req := vkg.RegisterRequest{
		Hostname: c.hostname,
		Version:  c.version,
		Seekers:  c.seekers,
	}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(c.serverURI+"/api/clients/register", "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data vkg.RegisterResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode register response: %w", err)
	}

	c.clientID = result.Data.ClientID
	c.logger.Info("registered with server", "clientId", c.clientID)
	return nil
}

func (c *Client) sendHeartbeat() error {
	hb := vkg.Heartbeat{
		ClientID: c.clientID,
		Hostname: c.hostname,
		Version:  c.version,
		Seekers:  c.seekers,
		KeyCount: c.keyCount.Load(),
	}
	b, err := json.Marshal(hb)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(c.serverURI+"/api/clients/heartbeat", "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Client) reportMatch(s seekerStatus) error {
	t := c.currentTarget()

	m := vkg.Match{
		ClientID:             c.clientID,
		Hostname:             c.hostname,
		SeekerID:             s.sid,
		MatchString:          s.matchString,
		MatchedAuthorizedKey: s.matchedAuthorizedKey,
		MatchedFingerprint:   s.matchedFingerprint,
		Key: vkg.Key{
			PrivateKey:       s.key.PrivateKey,
			PublicKey:        s.key.PublicKey,
			EncodedKey:       s.key.EncodedKey,
			PrivateString:    string(s.key.EncodedKey),
			AuthorizedString: s.key.AuthorizedKey,
			Fingerprint:      s.key.Fingerprint,
		},
	}
	if t != nil {
		m.TargetID = t.ID
	}

	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal match: %w", err)
	}

	resp, err := c.httpClient.Post(c.serverURI+"/api/matches", "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("post match: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.time" {
			return s.Value
		}
	}
	return "unknown"
}

// Run starts the client. It blocks until interrupted or a fatal error occurs.
func Run(ctx context.Context, l *slog.Logger, stdout io.Writer, stderr io.Writer, getenv func(string) string, args []string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	myFlags := FlagSet()
	if err := myFlags.Parse(args); err != nil {
		return err
	}

	if val := getenv("VKG_SERVER_URI"); val != "" {
		serverURI = val
	}

	hostname, _ := os.Hostname()

	c := &Client{
		logger:    l,
		serverURI: serverURI,
		hostname:  hostname,
		version:   buildVersion(),
		seekers:   numSeekers,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// Register with server
	if err := c.register(); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	// Fetch initial target
	if err := c.fetchTargetFull(); err != nil {
		return fmt.Errorf("initial target fetch: %w", err)
	}

	statusUpdates := make(chan seekerStatus, c.seekers*2)

	// Launch seekers
	var wg sync.WaitGroup
	for i := 1; i <= c.seekers; i++ {
		wg.Add(1)
		go func(sid int) {
			defer wg.Done()
			c.seeker(ctx, statusUpdates, sid)
		}(i)
	}

	stats := telemetry{
		launchStartTime: time.Now(),
	}

	statsTicker := time.NewTicker(5 * time.Second)
	defer statsTicker.Stop()
	targetTicker := time.NewTicker(20 * time.Second)
	defer targetTicker.Stop()
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

loop:
	for {
		select {
		case <-targetTicker.C:
			if err := c.fetchTargetFull(); err != nil {
				c.logger.Error("failed to fetch target", "error", err)
			}
		case <-heartbeatTicker.C:
			if err := c.sendHeartbeat(); err != nil {
				c.logger.Warn("heartbeat failed", "error", err)
			}
		case <-statsTicker.C:
			hitRate := float64(0)
			if stats.keyCount > 0 {
				hitRate = float64(stats.hitCount) / float64(stats.keyCount) * 100
			}
			c.logger.Debug("stats",
				"duration", time.Since(stats.launchStartTime),
				"keys", stats.keyCount,
				"hits", stats.hitCount,
				"hitRate", hitRate,
			)
		case s := <-statusUpdates:
			stats.keyCount += s.keyCount

			if s.key.Fingerprint != "" {
				stats.hitCount++
				c.logger.Info("match found", "s", s)

				if err := c.reportMatch(s); err != nil {
					c.logger.Warn("failed to report match", "error", err)
				}
			}
		case <-ctx.Done():
			break loop
		}
	}

	c.logger.Info("shutting down, waiting for seekers...")
	wg.Wait()
	c.logger.Info("shutdown complete")

	return nil
}
