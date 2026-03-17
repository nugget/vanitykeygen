package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nugget/vanitykeygen/pkg/keygen"
	"github.com/nugget/vanitykeygen/pkg/vkg"
)

// Version is set at build time via ldflags.
var Version = "dev"

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

	patterns      atomic.Value // stores []vkg.CompiledPattern
	keyCount      atomic.Int64
	lastHeartbeat time.Time
	lastKeyCount  int64
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

type compiledPattern struct {
	pattern            string
	re                 *regexp.Regexp
	matchFingerprint   bool
	matchAuthorizedKey bool
}

func (c *Client) currentPatterns() []vkg.CompiledPattern {
	v := c.patterns.Load()
	if v == nil {
		return nil
	}
	return v.([]vkg.CompiledPattern)
}

func (c *Client) seeker(ctx context.Context, statusUpdates chan<- seekerStatus, sid int) {
	logger := c.logger.With("sid", sid)
	logger.Info("seeker starting")

	var compiled []compiledPattern

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

		patterns := c.currentPatterns()
		if len(patterns) == 0 {
			time.Sleep(1 * time.Second)
			continue
		}

		// Recompile regexps when patterns change
		if !patternsMatch(compiled, patterns) {
			compiled = compiled[:0]
			for _, p := range patterns {
				re, err := regexp.Compile(p.Pattern)
				if err != nil {
					logger.Error("unable to compile regexp", "pattern", p.Pattern, "error", err)
					continue
				}
				compiled = append(compiled, compiledPattern{
					pattern:            p.Pattern,
					re:                 re,
					matchFingerprint:   p.MatchFingerprint,
					matchAuthorizedKey: p.MatchAuthorizedKey,
				})
			}
			patternStrs := make([]string, len(compiled))
			for i, cp := range compiled {
				patternStrs[i] = cp.pattern
			}
			logger.Info("patterns updated", "count", len(compiled), "patterns", patternStrs)
		}

		if len(compiled) == 0 {
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

		// Test key against all compiled patterns
		for _, cp := range compiled {
			var matchedFingerprint, matchedAuthorizedKey bool
			if cp.matchFingerprint {
				matchedFingerprint = cp.re.MatchString(k.Fingerprint)
			}
			if cp.matchAuthorizedKey {
				matchedAuthorizedKey = cp.re.MatchString(k.AuthorizedKey)
			}

			if matchedFingerprint || matchedAuthorizedKey {
				var matchString strings.Builder
				if matchedFingerprint {
					matchString.WriteString(cp.re.FindString(k.Fingerprint))
				}
				if matchedAuthorizedKey {
					matchString.WriteString(cp.re.FindString(k.AuthorizedKey))
				}

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

func patternsMatch(compiled []compiledPattern, patterns []vkg.CompiledPattern) bool {
	if len(compiled) != len(patterns) {
		return false
	}
	for i, cp := range compiled {
		if cp.pattern != patterns[i].Pattern ||
			cp.matchFingerprint != patterns[i].MatchFingerprint ||
			cp.matchAuthorizedKey != patterns[i].MatchAuthorizedKey {
			return false
		}
	}
	return true
}

// fetchTargets fetches compiled patterns from the server.
func (c *Client) fetchTargets() error {
	resp, err := c.httpClient.Get(c.serverURI + "/api/targets/active")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetch targets: server returned %s", resp.Status)
	}

	var result struct {
		Data []vkg.CompiledPattern `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode patterns: %w", err)
	}

	c.patterns.Store(result.Data)
	c.logger.Debug("patterns fetched", "count", len(result.Data))
	return nil
}

// stableID derives a deterministic client ID from the hostname so it
// persists across restarts without needing local state files.
func stableID(hostname string) string {
	h := sha256.Sum256([]byte("vkg-client:" + hostname))
	return hex.EncodeToString(h[:8])
}

func (c *Client) register() error {
	req := vkg.RegisterRequest{
		ClientID: stableID(c.hostname),
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("register: server returned %s", resp.Status)
	}

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
	now := time.Now()
	currentCount := c.keyCount.Load()

	var keyRate float64
	elapsed := now.Sub(c.lastHeartbeat).Seconds()
	if elapsed > 0 && c.lastHeartbeat != (time.Time{}) {
		keyRate = float64(currentCount-c.lastKeyCount) / elapsed
	}
	c.lastHeartbeat = now
	c.lastKeyCount = currentCount

	hb := vkg.Heartbeat{
		ClientID: c.clientID,
		Hostname: c.hostname,
		Version:  c.version,
		Seekers:  c.seekers,
		KeyRate:  keyRate,
		KeyCount: currentCount,
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("report match: server returned %s", resp.Status)
	}
	return nil
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

	l.Info("vkg client starting", "version", Version, "hostname", hostname, "seekers", numSeekers, "server", serverURI)

	c := &Client{
		logger:    l,
		serverURI: serverURI,
		hostname:  hostname,
		version:   Version,
		seekers:   numSeekers,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// Lower process priority so seekers are friendlier to other processes
	if err := setProcessNiceness(10); err != nil {
		c.logger.Warn("failed to set process niceness", "error", err)
	} else {
		c.logger.Info("process niceness set", "nice", 10)
	}

	// Register with server
	if err := c.register(); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	// Fetch initial target
	if err := c.fetchTargets(); err != nil {
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
			if err := c.fetchTargets(); err != nil {
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
