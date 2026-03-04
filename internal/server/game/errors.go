package game

import "errors"

var (
	ErrMatchNotFound        = errors.New("game: match not found")
	ErrMatchFull            = errors.New("game: match is full")
	ErrMatchNotJoinable     = errors.New("game: match is not joinable")
	ErrPlayerAlreadyInMatch = errors.New("game: player already in a match")
	ErrInvalidPlayerID      = errors.New("game: invalid player id")
	ErrInvalidPlayerName    = errors.New("game: invalid player name")
	ErrPlayerNotFound       = errors.New("game: player not found in match")
	ErrNotEnoughPlayers     = errors.New("game: not enough players to start match")
	ErrMatchAlreadyRunning  = errors.New("game: match already running")
	ErrMatchAlreadyEnded    = errors.New("game: match already ended")

	ErrMatchNotRunning     = errors.New("game: match is not running")
	ErrInvalidInputCommand = errors.New("game: invalid input command")
	ErrInvalidInputPayload = errors.New("game: invalid input payload")
	ErrDuplicateInput      = errors.New("game: duplicate input command")
	ErrInputRateLimited    = errors.New("game: input rate limited")
	ErrInputTooLateDropped = errors.New("game: input too late and dropped")
	ErrInputPlayerMismatch = errors.New("game: input player mismatch")
)
