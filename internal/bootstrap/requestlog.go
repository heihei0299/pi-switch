package bootstrap

import (
	"path/filepath"

	requestlogadapter "github.com/heihei0299/pi-switch/internal/adapters/outbound/requestlog"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/store"
	requestlogusecase "github.com/heihei0299/pi-switch/internal/usecase/requestlog"
)

func NewRequestLogService() requestlogusecase.Service {
	db, err := store.GetDB()
	var database requestlogusecase.Database = requestlogadapter.UnavailableDatabase{Err: err}
	if err == nil {
		database = requestlogadapter.NewSQLite(db)
	}
	legacy := requestlogadapter.NewLegacyFile(filepath.Join(filepath.Dir(config.ResolvePath()), "requests.log"))
	return requestlogusecase.New(database, legacy)
}
