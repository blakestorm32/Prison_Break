package networking

import (
	"encoding/json"
	"testing"

	"prison-break/internal/shared/protocol"
)

func FuzzEnvelopeDecodeAndTypedPayloadUnmarshalDoesNotPanic(f *testing.F) {
	f.Add([]byte(`{"version":1,"type":"join_game","payload":{"player_name":"alice"}}`))
	f.Add([]byte(`{"version":1,"type":"join_game","payload":{"spectator":true,"preferred_match_id":"match-000001"}}`))
	f.Add([]byte(`{"version":1,"type":"list_lobbies","payload":{"include_running":true}}`))
	f.Add([]byte(`{"version":1,"type":"request_replay","payload":{"match_id":"match-000001"}}`))
	f.Add([]byte(`{"version":1,"type":"player_input","payload":{"command":{"type":"reload"}}}`))
	f.Add([]byte(`{"version":1,"type":"ack_snapshot","payload":{"client_tick_id":10,"last_processed_client_seq":3}}`))
	f.Add([]byte(`{"version":1,"type":"unknown","payload":"x"}`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		var envelope protocol.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return
		}

		switch envelope.Type {
		case protocol.MsgJoinGame:
			_, _ = protocol.DecodePayload[protocol.JoinGameRequest](envelope)
		case protocol.MsgListLobbies:
			_, _ = protocol.DecodePayload[protocol.ListLobbiesRequest](envelope)
		case protocol.MsgPlayerInput:
			_, _ = protocol.DecodePayload[protocol.PlayerInputMessage](envelope)
		case protocol.MsgRequestReplay:
			_, _ = protocol.DecodePayload[protocol.ReplayLogRequest](envelope)
		case protocol.MsgAbilityUse:
			_, _ = protocol.DecodePayload[protocol.AbilityUseMessage](envelope)
		case protocol.MsgCardUse:
			_, _ = protocol.DecodePayload[protocol.CardUseMessage](envelope)
		case protocol.MsgPing:
			_, _ = protocol.DecodePayload[protocol.PingMessage](envelope)
		case protocol.MsgAckSnapshot:
			_, _ = protocol.DecodePayload[protocol.SnapshotAckMessage](envelope)
		default:
			// Ignore unsupported message types in fuzz harness.
		}
	})
}
