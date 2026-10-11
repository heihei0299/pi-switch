package main

import (
	"github.com/heihei0299/pi-switch/internal/server"
	webuiFS "github.com/heihei0299/pi-switch/webui"
)

func init() {
	server.SetWebUIAssets(webuiFS.FS)
}
