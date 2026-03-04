package constants

const (
	ProtocolVersion uint16 = 1

	MinPlayers uint8 = 5
	MaxPlayers uint8 = 12

	ServerTickRateHz  uint32 = 30
	TickDurationNanos int64  = 33_333_333
	MaxCatchupTicks   uint8  = 3

	MaxAcceptedCommandsPerPlayerPerTick uint8 = 8

	DefaultInterpolationBufferMS uint16 = 100
	DefaultCorrectionBlendMS     uint16 = 100

	DayPhaseDurationSeconds   uint16 = 300
	NightPhaseDurationSeconds uint16 = 120
	MaxDayNightCycles         uint8  = 6
)

const (
	// If correction error is above this threshold, clients snap immediately.
	PositionSnapThresholdTiles float32 = 0.20
)
