package models

import (
	"time"

	"gorm.io/gorm"
)

// Permission представляет разрешение в системе
type Permission struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля разрешения
	Name        string `json:"name" gorm:"uniqueIndex;not null;type:varchar(100)"` // Например: objects.create, users.read
	DisplayName string `json:"display_name" gorm:"not null;type:varchar(100)"`     // Человеко-читаемое название
	Description string `json:"description" gorm:"type:text"`                       // Описание разрешения
	Resource    string `json:"resource" gorm:"not null;type:varchar(50)"`          // objects, users, billing, etc.
	Action      string `json:"action" gorm:"not null;type:varchar(50)"`            // create, read, update, delete, etc.

	// Категория разрешения
	Category string `json:"category" gorm:"type:varchar(50)"` // management, warehouse, billing, reports

	// Статус
	IsActive bool `json:"is_active" gorm:"default:true"`
}

// TableName задает имя таблицы для модели Permission
func (Permission) TableName() string {
	return "permissions"
}

// Role представляет роль пользователя в системе
type Role struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля роли
	Name        string `json:"name" gorm:"uniqueIndex;not null;type:varchar(100)"` // admin, manager, tech, accountant
	DisplayName string `json:"display_name" gorm:"not null;type:varchar(100)"`     // Администратор, Менеджер, и т.д.
	Description string `json:"description" gorm:"type:text"`                       // Описание роли

	// Цвет для UI
	Color string `json:"color" gorm:"type:varchar(7)"` // HEX цвет для отображения в UI

	// Приоритет роли (чем больше число, тем выше приоритет)
	Priority int `json:"priority" gorm:"default:0"`

	// Статус
	IsActive bool `json:"is_active" gorm:"default:true"`
	IsSystem bool `json:"is_system" gorm:"default:false"` // Системная роль (нельзя удалить)

	// Связи
	Permissions []Permission `json:"permissions,omitempty" gorm:"many2many:role_permissions;"`
	Users       []User       `json:"users,omitempty" gorm:"foreignKey:RoleID"`
}

// TableName задает имя таблицы для модели Role
func (Role) TableName() string {
	return "roles"
}

// HasPermission проверяет, есть ли у роли определенное разрешение
func (r *Role) HasPermission(permissionName string) bool {
	for _, perm := range r.Permissions {
		if perm.Name == permissionName {
			return true
		}
	}
	return false
}

// HasPermissionFor проверяет, есть ли у роли разрешение для ресурса и действия
func (r *Role) HasPermissionFor(resource, action string) bool {
	for _, perm := range r.Permissions {
		if perm.Resource == resource && perm.Action == action {
			return true
		}
		// Проверяем wildcard разрешения
		if perm.Resource == resource && perm.Action == "*" {
			return true
		}
		if perm.Resource == "*" && perm.Action == action {
			return true
		}
		if perm.Resource == "*" && perm.Action == "*" {
			return true
		}
	}
	return false
}

// GetPermissionNames возвращает список имен разрешений роли
func (r *Role) GetPermissionNames() []string {
	names := make([]string, len(r.Permissions))
	for i, perm := range r.Permissions {
		names[i] = perm.Name
	}
	return names
}

// UserTemplate представляет шаблон пользователя с предустановленными настройками
type UserTemplate struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля шаблона
	Name        string `json:"name" gorm:"not null;type:varchar(100)"`
	Description string `json:"description" gorm:"type:text"`

	// Роль по умолчанию
	RoleID uint `json:"role_id" gorm:"not null"`
	Role   Role `json:"role" gorm:"foreignKey:RoleID"`

	// Дополнительные настройки (JSON)
	Settings string `json:"settings" gorm:"type:jsonb"` // Дополнительные настройки пользователя

	// Статус
	IsActive bool `json:"is_active" gorm:"default:true"`

	// Связи
	Users []User `json:"users,omitempty" gorm:"foreignKey:TemplateID"`
}

// TableName задает имя таблицы для модели UserTemplate
func (UserTemplate) TableName() string {
	return "user_templates"
}

