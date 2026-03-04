package game

import (
	"strings"
	"time"

	"prison-break/internal/gamecore/phase"
	"prison-break/internal/shared/constants"
)

type Config struct {
	MinPlayers    uint8
	MaxPlayers    uint8
	TickRateHz    uint32
	DaySeconds    uint16
	NightSeconds  uint16
	MaxCycles     uint8
	MatchIDPrefix string
}

func DefaultConfig() Config {
	return Config{
		MinPlayers:    constants.MinPlayers,
		MaxPlayers:    constants.MaxPlayers,
		TickRateHz:    constants.ServerTickRateHz,
		DaySeconds:    constants.DayPhaseDurationSeconds,
		NightSeconds:  constants.NightPhaseDurationSeconds,
		MaxCycles:     constants.MaxDayNightCycles,
		MatchIDPrefix: "match",
	}
}

func (c Config) normalized() Config {
	if c.MinPlayers == 0 {
		c.MinPlayers = constants.MinPlayers
	}
	if c.MaxPlayers == 0 {
		c.MaxPlayers = constants.MaxPlayers
	}
	if c.MinPlayers > c.MaxPlayers {
		c.MinPlayers = c.MaxPlayers
	}
	if c.TickRateHz == 0 {
		c.TickRateHz = constants.ServerTickRateHz
	}
	if c.DaySeconds == 0 {
		c.DaySeconds = constants.DayPhaseDurationSeconds
	}
	if c.NightSeconds == 0 {
		c.NightSeconds = constants.NightPhaseDurationSeconds
	}
	if c.MaxCycles == 0 {
		c.MaxCycles = constants.MaxDayNightCycles
	}

	c.MatchIDPrefix = strings.TrimSpace(c.MatchIDPrefix)
	if c.MatchIDPrefix == "" {
		c.MatchIDPrefix = "match"
	}

	return c
}

func (c Config) TickInterval() time.Duration {
	normalized := c.normalized()
	if normalized.TickRateHz == 0 {
		return time.Second
	}

	interval := time.Second / time.Duration(normalized.TickRateHz)
	if interval <= 0 {
		return time.Nanosecond
	}

	return interval
}

func (c Config) phaseConfig() phase.Config {
	normalized := c.normalized()
	return phase.Config{
		DayDurationTicks:   uint64(normalized.DaySeconds) * uint64(normalized.TickRateHz),
		NightDurationTicks: uint64(normalized.NightSeconds) * uint64(normalized.TickRateHz),
		MaxCycles:          normalized.MaxCycles,
	}
}
