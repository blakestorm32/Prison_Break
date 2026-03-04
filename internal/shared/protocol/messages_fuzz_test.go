package protocol

import (
	"encoding/json"
	"testing"

	"prison-break/internal/shared/model"
)

func FuzzDecodeJoinGameRequestDoesNotPanic(f *testing.F) {
	f.Add([]byte(`{"player_name":"alice","preferred_match_id":"match-000001"}`))
	f.Add([]byte(`{"spectator":true,"preferred_match_id":"match-000001"}`))
	f.Add([]byte(`{"player_name":"","client_build":"dev"}`))
	f.Add([]byte(`null`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		env := Envelope{
			Type:    MsgJoinGame,
			Payload: payload,
		}

		_, _ = DecodePayload[JoinGameRequest](env)
	})
}

func FuzzDecodePlayerInputMessageDoesNotPanic(f *testing.F) {
	validPayload, err := json.Marshal(PlayerInputMessage{
		Command: model.InputCommand{
			PlayerID:  "p1",
			ClientSeq: 1,
			Type:      model.CmdMoveIntent,
			Payload:   json.RawMessage(`{"move_x":1,"move_y":0}`),
		},
	})
	if err != nil {
		f.Fatalf("marshal seed payload: %v", err)
	}
	f.Add(validPayload)
	f.Add([]byte(`{"command":{"type":"drop_item","payload":{}}}`))
	f.Add([]byte(`[]`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		env := Envelope{
			Type:    MsgPlayerInput,
			Payload: payload,
		}

		_, _ = DecodePayload[PlayerInputMessage](env)
	})
}

func FuzzDecodeReplayLogRequestDoesNotPanic(f *testing.F) {
	f.Add([]byte(`{"match_id":"match-000001"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`"not-an-object"`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		env := Envelope{
			Type:    MsgRequestReplay,
			Payload: payload,
		}

		_, _ = DecodePayload[ReplayLogRequest](env)
	})
}
