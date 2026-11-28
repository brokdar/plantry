//go:build ignore

// calibrate prints the CPU seconds of a fixed workload (maps, sorting,
// hashing, allocation: what Go tests spend their time on), the best of three
// runs. scripts/test-budget.py scales the time budgets by this figure against
// the one recorded when they were set, so a slower or busier machine (GitHub's
// hosted runners differ by 1.7x between runs) does not fail them and a faster
// one does not hide a slowdown.
//
//	go run scripts/calibrate.go
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"slices"
	"syscall"
	"time"
)

func cpu() time.Duration {
	var u syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &u)
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}

var sink int

func work() {
	for round := range 160 {
		m := make(map[int][]byte, 4096)
		for i := range 20000 {
			b := make([]byte, 64)
			b[0] = byte(i + round)
			m[i%4096] = b
		}
		keys := make([]int, 0, 50000)
		for i := range 50000 {
			keys = append(keys, (i*7919+round)%100003)
		}
		slices.Sort(keys)
		h := sha256.New()
		for _, k := range keys[:20000] {
			h.Write([]byte{byte(k), byte(k >> 8)})
		}
		sink += len(m) + int(h.Sum(nil)[0])
	}
}

func main() {
	best := time.Duration(1 << 62)
	rounds := 3
	if len(os.Args) > 1 && os.Args[1] == "1" { // scripts/cpu-exec.py: one quick round
		rounds = 1
	}
	for range rounds {
		start := cpu()
		work()
		best = min(best, cpu()-start)
	}
	fmt.Printf("%.3f\n", best.Seconds())
}
