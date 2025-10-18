package types

// AxentaUserResponse структура для ответа от Axenta Cloud API
type AxentaUserResponse struct {
	AccountBlockingDatetime *string `json:"accountBlockingDatetime"`
	AccountName             string  `json:"accountName"`
	AccountType             string  `json:"accountType"`
	CreatorName             string  `json:"creatorName"`
	ID                      int     `json:"id"`
	LastLogin               string  `json:"lastLogin"`
	Name                    string  `json:"name"`
	Username                string  `json:"username"`
	Email                   string  `json:"email,omitempty"`
	AccountID               int     `json:"accountId,omitempty"`
	IsAdmin                 bool    `json:"isAdmin,omitempty"`
	IsActive                bool    `json:"isActive,omitempty"`
	Language                string  `json:"language,omitempty"`
	Timezone                int     `json:"timezone,omitempty"`
}
