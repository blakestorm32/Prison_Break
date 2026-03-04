package determinism

import (
	"encoding/binary"
	"hash/fnv"
)

const defaultSeed uint64 = 0x9e3779b97f4a7c15

// StreamRNG is a deterministic PRNG stream backed by splitmix64.
type StreamRNG struct {
	state uint64
}

func newStreamRNG(seed uint64) *StreamRNG {
	if seed == 0 {
		seed = defaultSeed
	}

	return &StreamRNG{state: seed}
}

func (r *StreamRNG) NextUint64() uint64 {
	r.state += defaultSeed
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *StreamRNG) NextIntn(n int) int {
	if n <= 0 {
		panic("determinism: NextIntn called with n <= 0")
	}

	return int(r.NextUint64() % uint64(n))
}

type RNGStreams struct {
	matchSeed uint64
	streams   map[string]*StreamRNG
}

func NewRNGStreams(matchSeed uint64) *RNGStreams {
	return &RNGStreams{
		matchSeed: matchSeed,
		streams:   make(map[string]*StreamRNG),
	}
}

func (r *RNGStreams) Stream(name string) *StreamRNG {
	if stream, ok := r.streams[name]; ok {
		return stream
	}

	seed := DeriveStreamSeed(r.matchSeed, name)
	stream := newStreamRNG(seed)
	r.streams[name] = stream
	return stream
}

func DeriveStreamSeed(matchSeed uint64, streamName string) uint64 {
	h := fnv.New64a()

	var seedBytes [8]byte
	binary.LittleEndian.PutUint64(seedBytes[:], matchSeed)
	_, _ = h.Write(seedBytes[:])
	_, _ = h.Write([]byte(streamName))

	seed := h.Sum64()
	if seed == 0 {
		return defaultSeed
	}

	return seed
}
