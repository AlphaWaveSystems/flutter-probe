//go:build race

package main

// raceEnabled is true under the race detector, which makes the patterns several times slower.
const raceEnabled = true
