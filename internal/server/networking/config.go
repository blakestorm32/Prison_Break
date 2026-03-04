package networking

import (
	"strings"
	"time"
)

type Config struct {
	ReadLimitBytes int64
	WriteTimeout   time.Duration
	PongTimeout    time.Duration
	PingInterval   time.Duration
	SendQueueDepth int

	RequireAuth   bool
	AuthSecret    string
	AuthClockSkew time.Duration
}

func DefaultConfig() Config {
	return Config{
		ReadLimitBytes: 1 << 20, // 1 MiB
		WriteTimeout:   5 * time.Second,
		PongTimeout:    60 * time.Second,
		PingInterval:   30 * time.Second,
		SendQueueDepth: 64,
		RequireAuth:    false,
		AuthClockSkew:  5 * time.Second,
	}
}

func (c Config) normalized() Config {
	if c.ReadLimitBytes <= 0 {
		c.ReadLimitBytes = DefaultConfig().ReadLimitBytes
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = DefaultConfig().WriteTimeout
	}
	if c.PongTimeout <= 0 {
		c.PongTimeout = DefaultConfig().PongTimeout
	}
	if c.PingInterval <= 0 {
		c.PingInterval = DefaultConfig().PingInterval
	}
	if c.SendQueueDepth <= 0 {
		c.SendQueueDepth = DefaultConfig().SendQueueDepth
	}
	c.AuthSecret = strings.TrimSpace(c.AuthSecret)
	if c.AuthClockSkew <= 0 {
		c.AuthClockSkew = DefaultConfig().AuthClockSkew
	}

	return c
}
