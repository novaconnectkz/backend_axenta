package api

import (
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// versionInfo собирается один раз при первом запросе (lazy init)
var (
	versionOnce sync.Once
	versionData gin.H
)

// VersionInfo подставляется из main.go при старте через ldflags. В dev (go run)
// fallback — `git rev-list --count HEAD` runtime.
type VersionInfo struct {
	CommitCount string
	CommitHash  string
}

// versionInfoProvider — зарегистрирован main.go, чтобы api package мог получать значения ldflag-vars.
var versionInfoProvider func() VersionInfo

// SetVersionInfoProvider регистрирует callback, отдающий значения из main.gitCommitCount/Hash.
func SetVersionInfoProvider(fn func() VersionInfo) {
	versionInfoProvider = fn
}

// resolveCommitCount: сначала ldflag-значение через provider, иначе runtime git rev-list
func resolveCommitCount() (string, string) {
	if versionInfoProvider != nil {
		v := versionInfoProvider()
		if v.CommitCount != "" && v.CommitCount != "0" {
			return v.CommitCount, v.CommitHash
		}
	}
	// dev-fallback: вызываем git
	count, hash := "0", "dev"
	if out, err := exec.Command("git", "rev-list", "--count", "HEAD").Output(); err == nil {
		count = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		hash = strings.TrimSpace(string(out))
	}
	return count, hash
}

// GetAppVersion отдаёт версию backend.
// GET /api/version (public, без auth — нужно фронту до логина для отображения в footer/login).
func GetAppVersion(c *gin.Context) {
	versionOnce.Do(func() {
		count, hash := resolveCommitCount()
		versionData = gin.H{
			"backend_commit_count": count,
			"backend_commit_hash":  hash,
		}
	})
	c.JSON(http.StatusOK, versionData)
}
