package catalog

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	domaincatalog "github.com/heihei0299/pi-switch/internal/domain/catalog"
)

const (
	FetchURLVar     = "PI_SWITCH_CATALOG_URL"
	CachePathVar    = "PI_SWITCH_CATALOG"
	DefaultFetchURL = "https://models.dev/api.json"
	TTL             = 24 * time.Hour
	FetchTimeout    = 15 * time.Second
)

type Store struct{}

func New() *Store { return &Store{} }

func fetchURL() string {
	if url := os.Getenv(FetchURLVar); url != "" {
		return url
	}
	return DefaultFetchURL
}

func CachePath() string {
	if path := os.Getenv(CachePathVar); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/pi-switch-models-dev.json"
	}
	return filepath.Join(home, ".pi-switch", "cache", "models-dev.json")
}

// Snapshot loads a fresh cache or refreshes it, falling back to a readable stale
// cache (or an empty snapshot with a warning) when models.dev is unavailable.
func (s *Store) Snapshot(ctx context.Context) (domaincatalog.Snapshot, bool, string, error) {
	if err := ctx.Err(); err != nil {
		return domaincatalog.Snapshot{}, false, "", err
	}
	path := CachePath()
	client := &http.Client{Timeout: FetchTimeout}
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < TTL {
		if data, err := os.ReadFile(path); err == nil {
			if snapshot, err := domaincatalog.ParseSnapshot(data); err == nil {
				return snapshot, false, "", nil
			}
		}
	}
	request, fetchErr := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL(), nil)
	var resp *http.Response
	if fetchErr == nil {
		resp, fetchErr = client.Do(request)
	}
	if fetchErr == nil {
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if fetchErr = statusErr(resp.StatusCode); fetchErr == nil {
			if snapshot, err := domaincatalog.ParseSnapshot(data); err == nil {
				_ = writeCache(path, data)
				return snapshot, false, "", nil
			} else {
				fetchErr = err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return domaincatalog.Snapshot{}, false, "", err
	}
	if data, readErr := os.ReadFile(path); readErr == nil {
		if snapshot, parseErr := domaincatalog.ParseSnapshot(data); parseErr == nil && !snapshot.Empty() {
			return snapshot, true, "models.dev refresh failed, using stale cache", nil
		}
	}
	if fetchErr == nil {
		fetchErr = errNoCache
	}
	return domaincatalog.Snapshot{}, false, "models.dev unavailable: " + fetchErr.Error(), nil
}

func writeCache(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func statusErr(code int) error {
	if code >= 200 && code < 300 {
		return nil
	}
	return errString("models.dev: unexpected status " + itoa(code))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

var errNoCache = errString("no cache and refresh failed")

type errString string

func (e errString) Error() string { return string(e) }
