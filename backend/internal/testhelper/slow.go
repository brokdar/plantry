package testhelper

import "testing"

// Slow skips t under -short. The gate's race tier runs -race -short and its
// no-race pass runs everything, so a slow single-goroutine test still runs
// once per gate. The reason says why the test costs this much and why it
// cannot be faster; every Slow test is listed in test-budget.json.
func Slow(t testing.TB, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skip("slow, runs without -short: " + reason)
	}
}
