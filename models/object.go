package models

import "gorm.io/gorm"

type Object struct {
	gorm.Model
	Name      string `gorm:"type:varchar(100)"`
	Latitude  float64
	Longitude float64
	IMEI      string `gorm:"unique"`
}
