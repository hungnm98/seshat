package admin

import (
	"log/slog"

	_ "github.com/GoAdminGroup/go-admin/adapter/gin"
	"github.com/GoAdminGroup/go-admin/engine"
	"github.com/GoAdminGroup/go-admin/modules/config"
	_ "github.com/GoAdminGroup/go-admin/modules/db/drivers/postgres"
	_ "github.com/GoAdminGroup/themes/adminlte"
	"github.com/gin-gonic/gin"
)

func SetupGoAdmin(router *gin.Engine, postgresDSN string, logger *slog.Logger) error {
	if postgresDSN == "" {
		logger.Info("skipping go-admin integration because SESHAT_POSTGRES_DSN is empty")
		return nil
	}
	eng := engine.Default()
	cfg := config.Config{
		Databases: config.DatabaseList{
			"default": {
				Driver: config.DriverPostgresql,
				Dsn:    postgresDSN,
			},
		},
		UrlPrefix: "admin/ui",
		Theme:     "adminlte",
		IndexUrl:  "/admin/",
		Debug:     false,
	}
	if err := eng.AddConfig(&cfg).Use(router); err != nil {
		return err
	}
	logger.Info("go-admin integration mounted", "prefix", "/admin/ui")
	return nil
}
