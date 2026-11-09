package main

import (
	"net/http"
	"sync"

	"github.com/showwin/speedtest-go/speedtest"
)

// newClientMu serializes speedtest client creation; see newClient.
var newClientMu sync.Mutex

// newClient creates a speedtest client with its own HTTP client.
//
// speedtest.New (v1.7.10) starts from the shared http.DefaultClient and sets
// its Transport to the new client, so every call rewires global state, and
// clients created by parallel tests would race on it and route requests
// through each other. Giving each client a private http.Client means nothing
// uses http.DefaultClient afterwards, and the mutex keeps the library's own
// write to it during construction from overlapping another.
func newClient(cfg *speedtest.UserConfig) *speedtest.Speedtest {
	if cfg == nil {
		cfg = &speedtest.UserConfig{UserAgent: speedtest.DefaultUserAgent}
	}
	newClientMu.Lock()
	defer newClientMu.Unlock()
	// WithDoer must come first: WithUserConfig wires the transport into
	// whichever http.Client the speedtest client holds at that point.
	return speedtest.New(speedtest.WithDoer(&http.Client{}), speedtest.WithUserConfig(cfg))
}
