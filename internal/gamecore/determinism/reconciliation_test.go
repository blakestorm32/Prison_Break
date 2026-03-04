package determinism

import (
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestDropAckedInputsNilAndEmpty(t *testing.T) {
	if out := DropAckedInputs(nil, 10); out != nil {
		t.Fatalf("expected nil output for nil input, got %#v", out)
	}

	empty := []model.InputCommand{}
	out := DropAckedInputs(empty, 10)
	if out != nil {
		t.Fatalf("expected nil output for empty input, got %#v", out)
	}
}

func TestDropAckedInputsAllDroppedWhenAckIsHighest(t *testing.T) {
	pending := []model.InputCommand{
		{ClientSeq: 1},
		{ClientSeq: 2},
		{ClientSeq: 3},
	}

	filtered := DropAckedInputs(pending, 3)
	if len(filtered) != 0 {
		t.Fatalf("expected all inputs to be dropped, got %d entries", len(filtered))
	}
}

func TestDropAckedInputsPreservesOriginalOrderForRemaining(t *testing.T) {
	pending := []model.InputCommand{
		{ClientSeq: 6},
		{ClientSeq: 2},
		{ClientSeq: 7},
		{ClientSeq: 5},
	}

	filtered := DropAckedInputs(pending, 4)

	got := make([]uint64, 0, len(filtered))
	for _, cmd := range filtered {
		got = append(got, cmd.ClientSeq)
	}

	want := []uint64{6, 7, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected preserved order among remaining commands, got=%v want=%v", got, want)
	}
}
