package utils

import (
	"log"
	"runtime/debug"
)

// GoSafe runs fn in a goroutine with panic recovery and logging.
func GoSafe(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[goroutine][panic] name=%s err=%v stack=%s", name, r, string(debug.Stack()))
			}
		}()
		fn()
	}()
}
