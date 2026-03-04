package determinism

import "testing"

func TestDeriveStreamSeedDeterministicAndDistinct(t *testing.T) {
	matchSeed := uint64(42)

	rolesA := DeriveStreamSeed(matchSeed, "roles")
	rolesB := DeriveStreamSeed(matchSeed, "roles")
	events := DeriveStreamSeed(matchSeed, "events")

	if rolesA != rolesB {
		t.Fatalf("expected identical derived seed for same stream name, got %d and %d", rolesA, rolesB)
	}
	if rolesA == events {
		t.Fatalf("expected distinct derived seeds for different stream names, both were %d", rolesA)
	}
}

func TestNewRNGStreamsCachesNamedStream(t *testing.T) {
	streams := NewRNGStreams(123)

	a := streams.Stream("roles")
	b := streams.Stream("roles")

	if a != b {
		t.Fatalf("expected cached stream pointer for same stream name")
	}
}

func TestRNGStreamsNamedStreamsAreIndependent(t *testing.T) {
	streams := NewRNGStreams(123)

	rolesFirst := streams.Stream("roles").NextUint64()
	eventsFirst := streams.Stream("events").NextUint64()

	if rolesFirst == eventsFirst {
		t.Fatalf("expected different first values for distinct named streams, both were %d", rolesFirst)
	}
}

func TestStreamRNGNextIntnPanicsOnNonPositive(t *testing.T) {
	testCases := []struct {
		name string
		n    int
	}{
		{name: "zero", n: 0},
		{name: "negative", n: -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stream := newStreamRNG(55)

			defer func() {
				if recover() == nil {
					t.Fatalf("expected panic for n=%d", tc.n)
				}
			}()

			_ = stream.NextIntn(tc.n)
		})
	}
}

func TestStreamRNGZeroSeedFallsBackToDefaultSeed(t *testing.T) {
	fromZero := newStreamRNG(0)
	fromDefault := newStreamRNG(defaultSeed)

	for i := 0; i < 5; i++ {
		a := fromZero.NextUint64()
		b := fromDefault.NextUint64()
		if a != b {
			t.Fatalf("expected zero-seeded stream to match default-seeded stream at sample %d", i)
		}
	}
}
