package netclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"prison-break/internal/shared/protocol"
)

func FetchLobbies(ctx context.Context, serverURL string, includeRunning bool) ([]protocol.LobbySummary, error) {
	return fetchLobbiesWithPreferences(ctx, serverURL, includeRunning, "", "", nil)
}

func FetchLobbiesWithAuth(
	ctx context.Context,
	serverURL string,
	includeRunning bool,
	authToken string,
) ([]protocol.LobbySummary, error) {
	return fetchLobbiesWithPreferences(ctx, serverURL, includeRunning, authToken, "", nil)
}

func FetchLobbiesWithPreferencesAndAuth(
	ctx context.Context,
	serverURL string,
	includeRunning bool,
	authToken string,
	preferredRegion string,
	regionLatencyMS map[string]uint16,
) ([]protocol.LobbySummary, error) {
	return fetchLobbiesWithPreferences(ctx, serverURL, includeRunning, authToken, preferredRegion, regionLatencyMS)
}

func fetchLobbiesWithPreferences(
	ctx context.Context,
	serverURL string,
	includeRunning bool,
	authToken string,
	preferredRegion string,
	regionLatencyMS map[string]uint16,
) ([]protocol.LobbySummary, error) {
	httpURL, err := wsURLToLobbiesHTTPURL(serverURL, includeRunning)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpURL, nil)
	if err != nil {
		return nil, fmt.Errorf("netclient: build lobby request: %w", err)
	}
	trimmedToken := strings.TrimSpace(authToken)
	if trimmedToken != "" {
		req.Header.Set("Authorization", "Bearer "+trimmedToken)
	}
	trimmedPreferredRegion := strings.ToLower(strings.TrimSpace(preferredRegion))
	if trimmedPreferredRegion != "" {
		query := req.URL.Query()
		query.Set("preferred_region", trimmedPreferredRegion)
		req.URL.RawQuery = query.Encode()
	}
	if len(regionLatencyMS) > 0 {
		serializedLatency := serializeRegionLatencyMap(regionLatencyMS)
		if serializedLatency != "" {
			query := req.URL.Query()
			query.Set("region_latency_ms", serializedLatency)
			req.URL.RawQuery = query.Encode()
		}
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("netclient: request lobbies: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("netclient: list lobbies returned status %d", resp.StatusCode)
	}

	var payload protocol.LobbyListMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("netclient: decode lobby list payload: %w", err)
	}
	return payload.Lobbies, nil
}

func serializeRegionLatencyMap(regionLatencyMS map[string]uint16) string {
	if len(regionLatencyMS) == 0 {
		return ""
	}

	normalized := make(map[string]uint16, len(regionLatencyMS))
	for region, latency := range regionLatencyMS {
		normalizedRegion := strings.ToLower(strings.TrimSpace(region))
		if normalizedRegion == "" || latency == 0 {
			continue
		}
		normalized[normalizedRegion] = latency
	}
	if len(normalized) == 0 {
		return ""
	}

	regions := make([]string, 0, len(normalized))
	for region := range normalized {
		regions = append(regions, region)
	}
	sort.Strings(regions)

	pairs := make([]string, 0, len(regions))
	for _, region := range regions {
		latency := normalized[region]
		pairs = append(pairs, fmt.Sprintf("%s:%d", region, latency))
	}
	return strings.Join(pairs, ",")
}

func wsURLToLobbiesHTTPURL(serverURL string, includeRunning bool) (string, error) {
	trimmed := strings.TrimSpace(serverURL)
	if trimmed == "" {
		return "", fmt.Errorf("netclient: server URL is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("netclient: parse server URL: %w", err)
	}

	switch parsed.Scheme {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	case "http", "https":
		// Keep as-is for flexibility in tests and tooling.
	default:
		return "", fmt.Errorf("netclient: unsupported server URL scheme %q", parsed.Scheme)
	}

	parsed.Path = path.Join("/", "lobbies")
	query := parsed.Query()
	if includeRunning {
		query.Set("include_running", "true")
	} else {
		query.Del("include_running")
	}
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}
