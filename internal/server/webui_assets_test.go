package server

import webuiFS "github.com/heihei0299/pi-switch/webui"

func init() {
	SetWebUIAssets(webuiFS.FS)
}
