package api

import (
	"backend_axenta/services"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Handlers для расширенных вкладок свойств Wialon-аккаунта.

func (api *WialonConnectionAPI) GetWialonAccountServices(c *gin.Context) {
	connID, userID, _, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	out, err := services.NewWialonAccountService().GetAccountServices(connID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"services": out})
}

func (api *WialonConnectionAPI) UpdateWialonAccountService(c *gin.Context) {
	connID, userID, companyID, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	var req services.UpdateWialonServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := services.NewWialonAccountService().UpdateAccountService(connID, userID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	invalidateAllAccountsCache(companyID)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (api *WialonConnectionAPI) GetWialonAccountAccessList(c *gin.Context) {
	connID, userID, _, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	out, err := services.NewWialonAccountService().GetResourceAccessList(connID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

func (api *WialonConnectionAPI) UpdateWialonAccountUserAccess(c *gin.Context) {
	connID, userID, _, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	targetUserID, err := strconv.ParseInt(c.Param("target_user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный target_user_id"})
		return
	}
	var req struct {
		AccessMask int64 `json:"access_mask"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := services.NewWialonAccountService().UpdateUserAccess(connID, userID, targetUserID, req.AccessMask); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (api *WialonConnectionAPI) GetWialonAccountCustomFields(c *gin.Context) {
	connID, userID, _, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	out, err := services.NewWialonAccountService().GetAccountCustomFields(connID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"fields": out})
}

func (api *WialonConnectionAPI) UpsertWialonAccountCustomField(c *gin.Context) {
	connID, userID, companyID, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	var req services.UpsertWialonCustomFieldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := services.NewWialonAccountService().UpsertCustomField(connID, userID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	invalidateAllAccountsCache(companyID)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (api *WialonConnectionAPI) DeleteWialonAccountCustomField(c *gin.Context) {
	connID, userID, companyID, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	fieldID, err := strconv.ParseInt(c.Param("field_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный field_id"})
		return
	}
	if err := services.NewWialonAccountService().DeleteCustomField(connID, userID, fieldID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	invalidateAllAccountsCache(companyID)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (api *WialonConnectionAPI) GetWialonAccountExtra(c *gin.Context) {
	connID, userID, _, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	out, err := services.NewWialonAccountService().GetAccountExtra(connID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func (api *WialonConnectionAPI) UpdateWialonAccountFTP(c *gin.Context) {
	connID, userID, _, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	var req services.UpdateFTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := services.NewWialonAccountService().UpdateFTP(connID, userID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (api *WialonConnectionAPI) UpdateWialonAccountEmailTemplate(c *gin.Context) {
	connID, userID, _, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	var req services.UpdateEmailTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := services.NewWialonAccountService().UpdateEmailTemplate(connID, userID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (api *WialonConnectionAPI) GetWialonAccountHistory(c *gin.Context) {
	connID, userID, _, ok := api.parseAccountPath(c)
	if !ok {
		return
	}
	fromUnix, _ := strconv.ParseInt(c.Query("from"), 10, 64)
	toUnix, _ := strconv.ParseInt(c.Query("to"), 10, 64)
	typeFilter, _ := strconv.Atoi(c.Query("type"))
	if fromUnix == 0 {
		fromUnix = time.Now().AddDate(0, 0, -30).Unix()
	}
	out, err := services.NewWialonAccountService().GetAccountHistory(connID, userID, fromUnix, toUnix, typeFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"records": out})
}
