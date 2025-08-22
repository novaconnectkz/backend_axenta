package main

import (
	"backend_axenta/models"
	"fmt"
)

func main() {
	company := models.Company{}
	fmt.Printf("Company ID type: %T\n", company.ID)
}
