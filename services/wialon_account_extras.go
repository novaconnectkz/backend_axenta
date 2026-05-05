package services

import (
	"backend_axenta/models"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Расширенные методы для свойств Wialon-аккаунта (вкладки CMS Manager):
// - Услуги (account/update_billing_service)
// - Доступ (user/update_item_access + core/get_items_access + список юзеров)
// - Произвольные поля (item/update_custom_field)
// - Дополнительно (item/update_ftp_property + item/update_email_template)
// - Статистика (account/get_account_history)

// ============================ УСЛУГИ ============================

type WialonServiceItem struct {
	Name     string `json:"name"`
	Type     int    `json:"type"`  // 1 = по требованию, 2 = периодическая
	Usage    int    `json:"usage"` // текущее использование
	MaxUsage int    `json:"max_usage"`
	Cost     string `json:"cost"`     // costTable string
	Interval int    `json:"interval"` // 0 нет, 1 час, 2 день, 3 нед, 4 мес
	Descr    string `json:"descr,omitempty"`
	Active   bool   `json:"active"` // вычисляется: maxUsage > 0
}

// GetAccountServices — список услуг с состоянием (из settings.plan.services).
func (s *WialonAccountService) GetAccountServices(connectionID uint, userID int64) ([]WialonServiceItem, error) {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return nil, err
	}
	resourceID, err := s.resolveResourceID(connectionID, userID)
	if err != nil {
		return nil, err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	body, err := s.callRaw(conn.Host, loginResp.Eid, "account/get_account_data", map[string]interface{}{
		"itemId": resourceID,
		"type":   2,
	})
	if err != nil {
		return nil, fmt.Errorf("get_account_data: %w", err)
	}
	var raw struct {
		Settings struct {
			Combined struct {
				Services map[string]struct {
					Type     int    `json:"type"`
					Usage    int    `json:"usage"`
					MaxUsage int    `json:"maxUsage"`
					Cost     string `json:"cost"`
					Interval int    `json:"interval"`
					Descr    string `json:"descr"`
				} `json:"services"`
			} `json:"combined"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	out := make([]WialonServiceItem, 0, len(raw.Settings.Combined.Services))
	for name, svc := range raw.Settings.Combined.Services {
		out = append(out, WialonServiceItem{
			Name:     name,
			Type:     svc.Type,
			Usage:    svc.Usage,
			MaxUsage: svc.MaxUsage,
			Cost:     svc.Cost,
			Interval: svc.Interval,
			Descr:    svc.Descr,
			Active:   svc.MaxUsage > 0 || svc.MaxUsage == -1,
		})
	}
	return out, nil
}

type UpdateWialonServiceRequest struct {
	Name      string `json:"name" binding:"required"`
	Type      int    `json:"type"` // 1 или 2
	Interval  int    `json:"interval"`
	CostTable string `json:"cost_table"` // см. формат /update_billing_service
}

// UpdateAccountService — account/update_billing_service.
func (s *WialonAccountService) UpdateAccountService(connectionID uint, userID int64, req UpdateWialonServiceRequest) error {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return err
	}
	resourceID, err := s.resolveResourceID(connectionID, userID)
	if err != nil {
		return err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	if _, err := s.callRaw(conn.Host, loginResp.Eid, "account/update_billing_service", map[string]interface{}{
		"itemId":       resourceID,
		"name":         req.Name,
		"type":         req.Type,
		"intervalType": req.Interval,
		"costTable":    req.CostTable,
	}); err != nil {
		return fmt.Errorf("update_billing_service: %w", err)
	}
	log.Printf("✅ Wialon service updated: resource=%d service=%s", resourceID, req.Name)
	return nil
}

// ============================ ДОСТУП ============================

type WialonAccessUser struct {
	UserID     int64  `json:"user_id"`
	UserName   string `json:"user_name"`
	AccessMask int64  `json:"access_mask"` // hex bitmask
}

// GetResourceAccessList — список пользователей и их accessMask к этому ресурсу.
// core/get_items_access по itemId=ресурс — возвращает map[userID]mask.
func (s *WialonAccountService) GetResourceAccessList(connectionID uint, userID int64) ([]WialonAccessUser, error) {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return nil, err
	}
	resourceID, err := s.resolveResourceID(connectionID, userID)
	if err != nil {
		return nil, err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	body, err := s.callRaw(conn.Host, loginResp.Eid, "core/check_accessors", map[string]interface{}{
		"items": []int64{resourceID},
		"flags": 1,
	})
	if err != nil {
		return nil, fmt.Errorf("check_accessors: %w", err)
	}
	// Ответ: { "<resource_id>": { "<user_id>": { "cacl": <mask>, "dacl": <mask> } } }
	var nested map[string]map[string]struct {
		Cacl json.Number `json:"cacl"`
		Dacl json.Number `json:"dacl"`
	}
	if err := json.Unmarshal(body, &nested); err != nil {
		return nil, fmt.Errorf("parse access: %w", err)
	}
	rawMap := map[string]int64{}
	for _, userMap := range nested {
		for uidStr, acls := range userMap {
			n, _ := acls.Cacl.Int64()
			if n == 0 {
				n, _ = acls.Dacl.Int64()
			}
			rawMap[uidStr] = n
		}
	}

	// Получим имена юзеров одним запросом
	out := make([]WialonAccessUser, 0, len(rawMap))
	for uidStr, mask := range rawMap {
		var uid int64
		fmt.Sscanf(uidStr, "%d", &uid)
		out = append(out, WialonAccessUser{UserID: uid, AccessMask: mask})
	}
	// Резолвим имена через search_items by ID (один батч)
	if len(out) > 0 {
		ids := make([]int64, 0, len(out))
		for _, u := range out {
			ids = append(ids, u.UserID)
		}
		nameMap := s.resolveUserNames(conn.Host, loginResp.Eid, ids)
		for i := range out {
			out[i].UserName = nameMap[out[i].UserID]
		}
	}
	return out, nil
}

func (s *WialonAccountService) resolveUserNames(host, eid string, ids []int64) map[int64]string {
	out := map[int64]string{}
	body, err := s.callRaw(host, eid, "core/search_items", map[string]interface{}{
		"spec": map[string]interface{}{
			"itemsType":     "user",
			"propName":      "sys_id",
			"propValueMask": "*",
			"sortType":      "sys_id",
		},
		"force": 1, "flags": 1, "from": 0, "to": 0,
	})
	if err != nil {
		return out
	}
	var resp struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return out
	}
	idSet := map[int64]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	for _, item := range resp.Items {
		if id, ok := item["id"].(float64); ok {
			if !idSet[int64(id)] {
				continue
			}
			if nm, ok := item["nm"].(string); ok {
				out[int64(id)] = nm
			}
		}
	}
	return out
}

// UpdateUserAccess — user/update_item_access.
func (s *WialonAccountService) UpdateUserAccess(connectionID uint, accountUserID, targetUserID int64, accessMask int64) error {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return err
	}
	resourceID, err := s.resolveResourceID(connectionID, accountUserID)
	if err != nil {
		return err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	if _, err := s.callRaw(conn.Host, loginResp.Eid, "user/update_item_access", map[string]interface{}{
		"userId":     targetUserID,
		"itemId":     resourceID,
		"accessMask": accessMask,
	}); err != nil {
		return fmt.Errorf("update_item_access: %w", err)
	}
	log.Printf("✅ Wialon access updated: resource=%d user=%d mask=0x%x", resourceID, targetUserID, accessMask)
	return nil
}

// ============================ ПРОИЗВОЛЬНЫЕ ПОЛЯ ============================

type WialonCustomField struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// GetAccountCustomFields — flds объекта (custom fields, flag 0x00000008).
func (s *WialonAccountService) GetAccountCustomFields(connectionID uint, userID int64) ([]WialonCustomField, error) {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return nil, err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	body, err := s.callRaw(conn.Host, loginResp.Eid, "core/search_item", map[string]interface{}{
		"id":    userID,
		"flags": 0x00000008, // custom fields
	})
	if err != nil {
		return nil, fmt.Errorf("search_item: %w", err)
	}
	var resp struct {
		Item struct {
			Flds map[string]struct {
				ID int64  `json:"id"`
				N  string `json:"n"`
				V  string `json:"v"`
			} `json:"flds"`
		} `json:"item"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	out := make([]WialonCustomField, 0, len(resp.Item.Flds))
	for _, f := range resp.Item.Flds {
		out = append(out, WialonCustomField{ID: f.ID, Name: f.N, Value: f.V})
	}
	return out, nil
}

type UpsertWialonCustomFieldRequest struct {
	ID    int64  `json:"id"` // 0 = create, >0 = update
	Name  string `json:"name" binding:"required"`
	Value string `json:"value"`
}

// UpsertCustomField — create/update через item/update_custom_field.
func (s *WialonAccountService) UpsertCustomField(connectionID uint, userID int64, req UpsertWialonCustomFieldRequest) error {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	mode := "create"
	if req.ID > 0 {
		mode = "update"
	}
	if _, err := s.callRaw(conn.Host, loginResp.Eid, "item/update_custom_field", map[string]interface{}{
		"itemId":   userID,
		"id":       req.ID,
		"callMode": mode,
		"n":        req.Name,
		"v":        req.Value,
	}); err != nil {
		return fmt.Errorf("update_custom_field: %w", err)
	}
	return nil
}

// DeleteCustomField — callMode=delete.
func (s *WialonAccountService) DeleteCustomField(connectionID uint, userID, fieldID int64) error {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	if _, err := s.callRaw(conn.Host, loginResp.Eid, "item/update_custom_field", map[string]interface{}{
		"itemId":   userID,
		"id":       fieldID,
		"callMode": "delete",
	}); err != nil {
		return fmt.Errorf("delete_custom_field: %w", err)
	}
	return nil
}

// ============================ ДОПОЛНИТЕЛЬНО (FTP + Email) ============================

type WialonExtraSettings struct {
	FTPEnabled bool   `json:"ftp_enabled"`
	FTPHost    string `json:"ftp_host"`
	FTPLogin   string `json:"ftp_login"`
	FTPPath    string `json:"ftp_path"`
	// password не возвращаем

	EmailEnabled bool   `json:"email_enabled"`
	EmailSubject string `json:"email_subject"`
	EmailBody    string `json:"email_body"`
}

// GetAccountExtra — FTP + email template (item/get_email_template + item flag 0x40 для FTP).
func (s *WialonAccountService) GetAccountExtra(connectionID uint, userID int64) (*WialonExtraSettings, error) {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return nil, err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	out := &WialonExtraSettings{}

	// FTP — flag 0x40 (connection_settings)
	body, err := s.callRaw(conn.Host, loginResp.Eid, "core/search_item", map[string]interface{}{
		"id":    userID,
		"flags": 0x00000040,
	})
	if err == nil {
		var resp struct {
			Item struct {
				FTP struct {
					Enabled int    `json:"enabled"`
					Host    string `json:"host"`
					Login   string `json:"login"`
					Path    string `json:"path"`
				} `json:"ftp"`
			} `json:"item"`
		}
		if json.Unmarshal(body, &resp) == nil {
			out.FTPEnabled = resp.Item.FTP.Enabled == 1
			out.FTPHost = resp.Item.FTP.Host
			out.FTPLogin = resp.Item.FTP.Login
			out.FTPPath = resp.Item.FTP.Path
		}
	}

	// Email template — get_email_template
	body, err = s.callRaw(conn.Host, loginResp.Eid, "item/get_email_template", map[string]interface{}{
		"itemId": userID,
	})
	if err == nil {
		var et struct {
			Subject string `json:"subject"`
			Body    string `json:"body"`
			// enabled flag — судя по UI отдельный
		}
		if json.Unmarshal(body, &et) == nil {
			out.EmailSubject = et.Subject
			out.EmailBody = et.Body
			out.EmailEnabled = et.Subject != "" || et.Body != ""
		}
	}
	return out, nil
}

type UpdateFTPRequest struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Login    string `json:"login"`
	Password string `json:"password,omitempty"` // опц., если пусто — не меняем
	Path     string `json:"path"`
}

// UpdateFTP — item/update_ftp_property.
func (s *WialonAccountService) UpdateFTP(connectionID uint, userID int64, req UpdateFTPRequest) error {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	params := map[string]interface{}{
		"itemId":  userID,
		"host":    req.Host,
		"login":   req.Login,
		"path":    req.Path,
		"enabled": enabled,
	}
	if req.Password != "" {
		params["password"] = req.Password
	}
	if _, err := s.callRaw(conn.Host, loginResp.Eid, "item/update_ftp_property", params); err != nil {
		return fmt.Errorf("update_ftp_property: %w", err)
	}
	return nil
}

type UpdateEmailTemplateRequest struct {
	Enabled bool   `json:"enabled"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// UpdateEmailTemplate — item/update_email_template.
func (s *WialonAccountService) UpdateEmailTemplate(connectionID uint, userID int64, req UpdateEmailTemplateRequest) error {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	if _, err := s.callRaw(conn.Host, loginResp.Eid, "item/update_email_template", map[string]interface{}{
		"itemId":  userID,
		"subject": req.Subject,
		"body":    req.Body,
	}); err != nil {
		return fmt.Errorf("update_email_template: %w", err)
	}
	return nil
}

// ============================ СТАТИСТИКА ============================

type WialonHistoryEntry struct {
	Time    int64  `json:"time"`
	Service string `json:"service"`
	Cost    string `json:"cost"`
	Counter int    `json:"counter"`
	Descr   string `json:"descr"`
}

// GetAccountHistory — account/get_account_history.
// API ожидает {itemId, days, tz}. Считаем days как (now-fromTime) в днях.
// type_: 0=все, 1=пополнение, 2=списание (фильтруем на нашей стороне).
func (s *WialonAccountService) GetAccountHistory(connectionID uint, userID, fromTime, toTime int64, typeFilter int) ([]WialonHistoryEntry, error) {
	conn, err := s.getConn(connectionID)
	if err != nil {
		return nil, err
	}
	resourceID, err := s.resolveResourceID(connectionID, userID)
	if err != nil {
		return nil, err
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	if fromTime == 0 {
		fromTime = time.Now().AddDate(0, 0, -30).Unix()
	}
	if toTime == 0 {
		toTime = time.Now().Unix()
	}
	days := int((time.Now().Unix() - fromTime) / 86400)
	if days < 1 {
		days = 1
	}
	body, err := s.callRaw(conn.Host, loginResp.Eid, "account/get_account_history", map[string]interface{}{
		"itemId": resourceID,
		"days":   days,
		"tz":     0,
	})
	if err != nil {
		return nil, fmt.Errorf("get_account_history: %w", err)
	}
	var resp struct {
		Records []struct {
			T int64  `json:"t"`
			S string `json:"s"`
			C string `json:"c"`
			N int    `json:"n"`
			D string `json:"d"`
		} `json:"records"`
	}
	// иногда возвращается массив прямо
	if err := json.Unmarshal(body, &resp); err != nil {
		var arr []struct {
			T int64  `json:"t"`
			S string `json:"s"`
			C string `json:"c"`
			N int    `json:"n"`
			D string `json:"d"`
		}
		if err := json.Unmarshal(body, &arr); err == nil {
			out := make([]WialonHistoryEntry, len(arr))
			for i, r := range arr {
				out[i] = WialonHistoryEntry{Time: r.T, Service: r.S, Cost: r.C, Counter: r.N, Descr: r.D}
			}
			return out, nil
		}
		return nil, fmt.Errorf("parse history: %w", err)
	}
	out := make([]WialonHistoryEntry, len(resp.Records))
	for i, r := range resp.Records {
		out[i] = WialonHistoryEntry{Time: r.T, Service: r.S, Cost: r.C, Counter: r.N, Descr: r.D}
	}
	return out, nil
}

// markused models pkg
var _ = models.WialonObjectStat{}
