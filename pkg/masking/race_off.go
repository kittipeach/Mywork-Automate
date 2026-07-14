//go:build !race

package masking

// raceEnabled is false in normal (non-race) builds, so wall-clock performance
// assertions are enforced.
const raceEnabled = false
