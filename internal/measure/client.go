package measure

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// newClientMu serializes speedtest client creation; see newClient.
var newClientMu sync.Mutex

// newClient creates a speedtest client with its own HTTP client.
//
// speedtest.New starts from the shared http.DefaultClient and sets its
// Transport to the new client, so every call rewires global state, and
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

// User is what Speedtest.net knows about this connection.
type User struct {
	ISP      string
	IP       string
	Lat, Lon float64
	Located  bool // Lat and Lon are known
}

// FetchUser looks up your internet provider, public IP and approximate
// location. It returns an empty User if the lookup fails.
func FetchUser(ctx context.Context) User {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	u, err := newClient(nil).FetchUserInfoContext(ctx)
	if err != nil || u == nil {
		return User{}
	}
	me := User{ISP: u.Isp, IP: u.IP}
	lat, errLat := strconv.ParseFloat(u.Lat, 64)
	lon, errLon := strconv.ParseFloat(u.Lon, 64)
	if errLat == nil && errLon == nil && (lat != 0 || lon != 0) {
		me.Lat, me.Lon, me.Located = lat, lon, true
	}
	return me
}
