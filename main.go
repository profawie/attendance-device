// attendance-device: a demo "push" server for attendance terminals
// (example: FingerTec Face ID 5) that speak the iclock/ADMS push protocol.
//
// FOR EDUCATIONAL PURPOSES ONLY. Not for production:
// no authentication, data is kept in memory only.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"smc.com/att/internal/api"
	"smc.com/att/internal/iclock"
	"smc.com/att/internal/logging"
	"smc.com/att/internal/store"
)

func main() {
	addr := env("ATT_ADDR", ":8081")
	logFile := env("ATT_LOG_FILE", "logs/traffic.log")

	logger, closeLog, err := logging.New(logFile)
	if err != nil {
		slog.Error("cannot open log file", "err", err)
		os.Exit(1)
	}
	defer closeLog()

	// Empty ATT_ALLOWED_SN = accept every device (demo mode).
	var allowed []string
	if v := strings.TrimSpace(os.Getenv("ATT_ALLOWED_SN")); v != "" {
		allowed = strings.Split(v, ",")
	}
	st := store.New(allowed, 90*time.Second)

	mux := http.NewServeMux()
	iclock.Register(mux, st, logger)
	api.Register(mux, st)

	srv := &http.Server{
		Addr:              addr,
		Handler:           logging.Middleware(logger, mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("attendance-device started", "addr", addr, "log_file", logFile, "allowed_sn", allowed)
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
