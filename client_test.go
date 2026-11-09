package main

import (
	"sync"
	"testing"
)

// Creating clients from many goroutines at once must be safe. Run with -race
// (as CI does) to check; without the mutex in newClient this test races.
func TestNewClientConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if newClient(nil) == nil {
				t.Error("newClient returned nil")
			}
		}()
	}
	wg.Wait()
}
