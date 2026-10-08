package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/egoist/mygo"
	"github.com/heihei0299/pi-switch/internal/server"
)

const managementUIScheme = "pi-switch-ui"

func registerManagementUI() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	host := hex.EncodeToString(nonce[:])
	if err := mygo.Protocol.Handle(managementUIScheme, managementUIHandler(host, server.NewMgmtRouter())); err != nil {
		return "", err
	}
	return managementUIScheme + "://" + host + "/", nil
}

// MyGo trusts every page in a registered scheme, so the random host makes the
// management API address specific to this app process.
func managementUIHandler(host string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Custom-scheme WebViews may send "null"; the random host is still required.
		if r.Host != host || (origin != "" && origin != "null" && origin != managementUIScheme+"://"+host) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
