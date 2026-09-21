package server

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/store"
)

func TestLogRequestInsertFailureIsObservable(t *testing.T) {
	dir := writeLegacyTestEnv(t, "")
	t.Cleanup(store.Close)
	db, err := store.GetDB()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_request_insert BEFORE INSERT ON requests BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	oldWriter := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(oldWriter)

	logRequest("provider", "model", true, 1, 1, 0, 0, nil, "", "", 1, http.StatusOK, "", "")
	if !strings.Contains(output.String(), "request log insert") {
		t.Fatalf("insert failure was not logged: %q", output.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "requests.log")); err != nil {
		t.Fatalf("legacy fallback log missing: %v", err)
	}
}
