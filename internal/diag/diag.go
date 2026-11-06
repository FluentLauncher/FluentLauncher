// Package diag records startup timing and memory for the spike.
package diag

import (
	"fmt"
	"log"
	"runtime"
	"time"
)

var started = time.Now()

// Init sets the log format and records the first mark.
func Init() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	Mark("process start")
}

// Mark logs a named instant since Init.
func Mark(name string) {
	log.Printf("[diag] %-24s +%dms", name, time.Since(started).Milliseconds())
}

// Mem returns heap and runtime memory for the spike footer.
func Mem() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("heap %.1fMB / sys %.1fMB", float64(m.HeapAlloc)/(1<<20), float64(m.Sys)/(1<<20))
}
