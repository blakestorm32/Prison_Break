package main

import (
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"prison-break/internal/client/netclient"
	"prison-break/internal/client/render"
	"prison-break/internal/shared/crashreport"
	"prison-break/internal/shared/model"
)

const (
	defaultWindowWidth  = 1280
	defaultWindowHeight = 720
)

var buildVersion = "dev"

func main() {
	defer func() {
		if crashreport.ReportRecoveredPanic("client", recover(), debug.Stack()) {
			os.Exit(1)
		}
	}()

	cfg := loadRunConfigFromEnv()
	log.Printf("starting client build=%s", buildVersion)

	ebiten.SetWindowSize(defaultWindowWidth, defaultWindowHeight)
	ebiten.SetWindowTitle("Prison Break")
	app := render.NewClientApp(render.ClientAppConfig{
		ScreenWidth:   defaultWindowWidth,
		ScreenHeight:  defaultWindowHeight,
		SessionConfig: cfg.session,
	})
	if err := ebiten.RunGame(app); err != nil {
		log.Fatalf("client exited with error: %v", err)
	}
}

type runConfig struct {
	session netclient.SessionConfig
}

func loadRunConfigFromEnv() runConfig {
	defaultSession := netclient.DefaultSessionConfig()

	serverURL := strings.TrimSpace(os.Getenv("PRISON_SERVER_WS_URL"))
	if serverURL == "" {
		serverURL = defaultSession.ServerURL
	}

	playerName := strings.TrimSpace(os.Getenv("PRISON_PLAYER_NAME"))
	playerID := strings.TrimSpace(os.Getenv("PRISON_PLAYER_ID"))
	preferredMatchID := strings.TrimSpace(os.Getenv("PRISON_PREFERRED_MATCH_ID"))
	preferredRegion := strings.TrimSpace(os.Getenv("PRISON_PREFERRED_REGION"))
	regionLatencyMS := parseRegionLatencyEnv(os.Getenv("PRISON_REGION_LATENCY_MS"))
	spectatorFollowPlayerID := strings.TrimSpace(os.Getenv("PRISON_SPECTATOR_FOLLOW_PLAYER"))
	spectatorFollowSlot := parseUint8Env("PRISON_SPECTATOR_FOLLOW_SLOT")
	authToken := strings.TrimSpace(os.Getenv("PRISON_SESSION_TOKEN"))
	spectator := parseBoolEnv("PRISON_SPECTATOR")

	return runConfig{
		session: netclient.SessionConfig{
			ServerURL:               serverURL,
			PlayerName:              playerName,
			PlayerID:                model.PlayerID(playerID),
			PreferredMatchID:        model.MatchID(preferredMatchID),
			PreferredRegion:         preferredRegion,
			RegionLatencyMS:         regionLatencyMS,
			Spectator:               spectator,
			SpectatorFollowPlayerID: model.PlayerID(spectatorFollowPlayerID),
			SpectatorFollowSlot:     spectatorFollowSlot,
			AuthToken:               authToken,
			HandshakeTimeout:        defaultSession.HandshakeTimeout,
			WriteTimeout:            defaultSession.WriteTimeout,
			SendQueueDepth:          defaultSession.SendQueueDepth,
		},
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

func parseRegionLatencyEnv(raw string) map[string]uint16 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	entries := strings.Split(trimmed, ",")
	out := make(map[string]uint16, len(entries))
	for _, entry := range entries {
		token := strings.TrimSpace(entry)
		if token == "" {
			continue
		}

		pair := strings.SplitN(token, "=", 2)
		if len(pair) != 2 {
			continue
		}

		region := strings.ToLower(strings.TrimSpace(pair[0]))
		if region == "" {
			continue
		}
		value := strings.TrimSpace(pair[1])
		if value == "" {
			continue
		}

		parsed, err := strconv.ParseUint(value, 10, 16)
		if err != nil {
			continue
		}
		latency := uint16(parsed)
		if latency == 0 {
			continue
		}

		out[region] = latency
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func parseUint8Env(key string) uint8 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0
	}
	return uint8(parsed)
}
