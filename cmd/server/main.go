package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"prison-break/internal/server/game"
	"prison-break/internal/server/networking"
	"prison-break/internal/server/persistence"
	"prison-break/internal/shared/crashreport"
)

var buildVersion = "dev"

func main() {
	defer func() {
		if crashreport.ReportRecoveredPanic("server", recover(), debug.Stack()) {
			os.Exit(1)
		}
	}()

	manager := game.NewManager(game.DefaultConfig())
	defer manager.Close()

	persistencePath := strings.TrimSpace(os.Getenv("PRISON_PERSIST_PATH"))
	if persistencePath == "" {
		persistencePath = "artifacts/server_persistence.json"
	}
	store, err := persistence.Open(persistence.Config{
		Path: persistencePath,
	})
	if err != nil {
		log.Fatalf("failed to initialize persistence store: %v", err)
	}
	manager.BindPersistence(store)
	log.Printf("persistence enabled at %s", persistencePath)

	netConfig := networking.DefaultConfig()
	netConfig.RequireAuth = parseBoolEnv("PRISON_AUTH_REQUIRED")
	netConfig.AuthSecret = strings.TrimSpace(os.Getenv("PRISON_AUTH_SECRET"))
	if rawClockSkew := strings.TrimSpace(os.Getenv("PRISON_AUTH_CLOCK_SKEW")); rawClockSkew != "" {
		parsedClockSkew, parseErr := time.ParseDuration(rawClockSkew)
		if parseErr != nil {
			log.Fatalf("invalid PRISON_AUTH_CLOCK_SKEW duration %q: %v", rawClockSkew, parseErr)
		}
		netConfig.AuthClockSkew = parsedClockSkew
	}
	if netConfig.RequireAuth && strings.TrimSpace(netConfig.AuthSecret) == "" {
		log.Fatal("PRISON_AUTH_REQUIRED=true requires PRISON_AUTH_SECRET to be set")
	}

	wsServer := networking.NewServer(manager, netConfig)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsServer.HandleWebSocket)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/lobbies", wsServer.HandleLobbiesHTTP)
	mux.HandleFunc("/admin", wsServer.HandleAdminHTTP)
	mux.HandleFunc("/admin/", wsServer.HandleAdminHTTP)

	addr := strings.TrimSpace(os.Getenv("PRISON_SERVER_ADDR"))
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("prison-break server build=%s listening on %s", buildVersion, addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server exited with error: %v", err)
	}
}

func parseBoolEnv(key string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
