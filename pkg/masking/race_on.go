//go:build race

package masking

// raceEnabled reports whether the binary was built with the race detector.
// Wall-clock performance assertions are unreliable under -race (5-10x overhead),
// so they are relaxed when this is true.
const raceEnabled = true
