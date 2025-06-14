package server

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/nugget/vanitykeygen/pkg/vkg"

	"github.com/gin-gonic/gin"
)

var (
	logger      *slog.Logger
	logLevel    *slog.LevelVar
	target      vkg.Target
	matchLogger *slog.Logger

	listenPort    int
	listenAddress string
	matchLogFile  string
	//keyDirectory string
)

func FlagSet() *flag.FlagSet {
	f := flag.NewFlagSet("server", flag.ExitOnError)

	f.IntVar(&listenPort, "p", 8080, "Specifies the port on which the server listens for connections")
	f.StringVar(&listenAddress, "b", "", "Bind this address on the local machine when listening for connections (default '' for all addresses)")
	f.StringVar(&matchLogFile, "l", "matchfile.log", "Log successful matches to this file")
	//f.StringVar(&keyDirectory), "keydir", "keys", "Store all matched keys in this location")

	return f
}

func getTarget(c *gin.Context) {
	c.JSON(http.StatusOK, target)
}

func postMatch(c *gin.Context) {
	var m vkg.Match

	decoder := json.NewDecoder(c.Request.Body)
	err := decoder.Decode(&m)
	if err != nil {
		logger.Error("Decoder Failed", "error", err)
	}

	logger.Info("received match",
		"hostname", m.Hostname,
		"seekerID", m.SeekerID,
		"authKey", m.Key.AuthorizedString,
		"finger", m.Key.Fingerprint,
	)

	matchLogger.Info("match reported", "match", m)
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

	target.MatchString = `(?i)[\/\+](nugget|slacker|wheelsdown|hollowoak|ferrari|porsche|gt3rs|portofino|longhorn|miata|equiraptor|nugget-info|vanitykey|vanity-nugget)=?$`
	val := getenv("VKG_TARGET")
	if val != "" {
		target.MatchString = val
	}

	matchFile, err := os.OpenFile(matchLogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer matchFile.Close()
	matchLogger = slog.New(slog.NewJSONHandler(matchFile, nil))
	logger.Info("Logging matches to file", "matchLogFile", matchLogFile)

	go func() {
		r := setupRouter()

		r.GET("/target", getTarget)
		r.POST("/match", postMatch)

		// Listen and Server in 0.0.0.0:8080
		r.Run(":8080")
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
