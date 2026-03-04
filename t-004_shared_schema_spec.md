# T-004 Shared Protocol and State Schema Spec

Status: Draft v1 (implementation baseline)  
Related roadmap ticket: `T-004` in `roadmap_tickets.md`

## Goal

Define shared, serializable schemas used by both server and client for:
- authoritative game state
- full and delta snapshots
- client input commands
- network message envelopes and payloads

## Canonical Files

- `internal/shared/constants/simulation.go`
- `internal/shared/model/types.go`
- `internal/shared/model/state.go`
- `internal/shared/model/input.go`
- `internal/shared/protocol/messages.go`

## Serialization Contract (v1)

1. Transport payload schema is JSON-tagged Go structs.
2. Protocol envelope includes `version`, `type`, optional `request_id`, optional `match_id`, optional `player_id`, and raw `payload`.
3. `constants.ProtocolVersion` is the single protocol version source.
4. Message payloads are decoded by envelope `type`.

## Model Contract

1. `model.GameState` is the canonical full state container.
2. `model.Snapshot` supports:
- full snapshot (`kind=full`, `state` set)
- delta snapshot (`kind=delta`, `delta` set, `base_tick_id` optional)
3. Snapshot metadata includes `tick_id`.
4. Snapshot ack path includes `player_acks[].last_processed_client_seq`.
5. `model.InputCommand` includes `player_id`, `client_seq`, optional `ingress_seq`, optional `target_tick`, `type`, and raw payload.

## Determinism Constraints

1. Slice fields representing ordered entities must be serialized in deterministic ID order.
2. Unordered map fields are intentionally avoided in shared state and snapshot schemas.
3. Input idempotency key is `(player_id, client_seq)`.

## Protocol Message Families

Client-to-server:
- `join_game`
- `leave_match`
- `player_input`
- `ability_use`
- `card_use`
- `ping`
- `ack_snapshot`

Server-to-client:
- `join_accepted`
- `player_joined`
- `player_left`
- `game_start`
- `snapshot`
- `phase_change`
- `game_over`
- `pong`
- `error`

## Notes

- v1 favors schema clarity over wire compactness; protobuf can be introduced later without changing the model intent.
- These schemas are baseline contracts for `T-005` determinism tests and upcoming server/client integration tickets.
