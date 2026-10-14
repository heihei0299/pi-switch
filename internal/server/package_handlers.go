package server

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	packageusecase "github.com/heihei0299/pi-switch/internal/usecase/packages"
)

func handlePackagesList(c *gin.Context) {
	out, err := bootstrap.NewPackagesService().ListInstalled()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"packages": out})
}

func handlePackageAdd(c *gin.Context) {
	var body struct {
		Spec    string `json:"spec"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Spec) == "" {
		c.JSON(400, gin.H{"error": "spec required"})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	if err := bootstrap.NewPackagesService().Add(packageusecase.AddCommand{Spec: body.Spec, Enabled: enabled}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "id": strings.TrimSpace(body.Spec)})
}

func handlePackageImport(c *gin.Context) {
	result, err := bootstrap.NewPackagesService().ImportPiPackages()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if result.Status == packageusecase.StatusNotFound {
		c.JSON(404, gin.H{"error": result.Message, "ok": result.OK, "count": result.Count, "status": result.Status, "message": result.Message, "warnings": result.Warnings})
		return
	}
	c.JSON(200, result)
}

func handlePackageGet(c *gin.Context) {
	pkg, err := bootstrap.NewPackagesService().GetInstalled(c.Param("id"))
	if err != nil {
		if errors.Is(err, packageusecase.ErrPackageNotFound) {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, pkg)
}

func handlePackageDelete(c *gin.Context) {
	id := strings.TrimPrefix(c.Param("id"), "/")
	decoded, err := url.PathUnescape(id)
	if err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("invalid package id: %v", err)})
		return
	}
	id = decoded
	if id == "" {
		c.JSON(400, gin.H{"error": "package id is required"})
		return
	}
	if err := bootstrap.NewPackagesService().DeleteInstalled(id); err != nil {
		if errors.Is(err, packageusecase.ErrPackageNotFound) {
			c.JSON(404, gin.H{"error": "package not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func handlePackageToggle(c *gin.Context) {
	enabled, err := bootstrap.NewPackagesService().ToggleInstalled(c.Param("id"))
	if err != nil {
		if errors.Is(err, packageusecase.ErrPackageNotFound) {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "enabled": enabled})
}
