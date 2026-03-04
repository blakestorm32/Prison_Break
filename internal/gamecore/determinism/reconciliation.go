package determinism

import "prison-break/internal/shared/model"

// DropAckedInputs removes pending local inputs acknowledged by server snapshots.
func DropAckedInputs(pending []model.InputCommand, lastProcessedClientSeq uint64) []model.InputCommand {
	if len(pending) == 0 {
		return nil
	}

	filtered := make([]model.InputCommand, 0, len(pending))
	for _, cmd := range pending {
		if cmd.ClientSeq <= lastProcessedClientSeq {
			continue
		}

		filtered = append(filtered, cmd)
	}

	return filtered
}
