package server

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/packages"
	"github.com/heihei0299/pi-switch/internal/piagent"
)

func PiSwitchDBPath() string { return packages.PiSwitchDBPath() }

func ListInstalledPackages() ([]map[string]interface{}, error) {
	return packages.ListInstalledPackages()
}

func AddInstalledPackage(spec string, enabled bool) error {
	return packages.AddInstalledPackage(spec, enabled)
}

func GetInstalledPackage(id string) (map[string]interface{}, error) {
	return packages.GetInstalledPackage(id)
}

func DeleteInstalledPackage(id string) error { return packages.DeleteInstalledPackage(id) }

func ToggleInstalledPackage(id string) (bool, error) { return packages.ToggleInstalledPackage(id) }

func ImportPiPackages() (piagent.ImportResult, error) { return packages.ImportPiPackages() }

var ErrPackageNotFound = packages.ErrPackageNotFound

func handlePackagesList(c *gin.Context) {
	out, err := ListInstalledPackages()
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
	if err := AddInstalledPackage(body.Spec, enabled); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "id": strings.TrimSpace(body.Spec)})
}
func handlePackageImport(c *gin.Context) {
	result, err := ImportPiPackages()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if result.Status == piagent.StatusNotFound {
		c.JSON(404, gin.H{"error": result.Message, "ok": result.OK, "count": result.Count, "status": result.Status, "message": result.Message, "warnings": result.Warnings})
		return
	}
	c.JSON(200, result)
}
func handlePackageGet(c *gin.Context) {
	m, err := GetInstalledPackage(c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrPackageNotFound) {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, m)
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
	if err := DeleteInstalledPackage(id); err != nil {
		if errors.Is(err, ErrPackageNotFound) {
			c.JSON(404, gin.H{"error": "package not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func handlePackageToggle(c *gin.Context) {
	enabled, err := ToggleInstalledPackage(c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrPackageNotFound) {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "enabled": enabled})
}
