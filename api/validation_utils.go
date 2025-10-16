package api

import (
	"strings"
)

// translateValidationError переводит сообщения ошибок валидации на русский язык
func translateValidationError(err error) string {
	errorMsg := err.Error()

	// Переводим основные сообщения валидации
	translations := map[string]string{
		// Email валидация
		"Field validation for 'Email' failed on the 'email' tag":    "Поле Email должно содержать корректный email адрес",
		"Field validation for 'Email' failed on the 'required' tag": "Поле Email обязательно для заполнения",

		// Username валидация
		"Field validation for 'Username' failed on the 'required' tag": "Поле Имя пользователя обязательно для заполнения",
		"Field validation for 'Username' failed on the 'min' tag":      "Имя пользователя должно содержать минимум 3 символа",
		"Field validation for 'Username' failed on the 'max' tag":      "Имя пользователя должно содержать максимум 50 символов",

		// Name валидация
		"Field validation for 'Name' failed on the 'required' tag": "Поле Имя обязательно для заполнения",

		// Password валидация
		"Field validation for 'Password' failed on the 'required' tag": "Поле Пароль обязательно для заполнения",
		"Field validation for 'Password' failed on the 'min' tag":      "Пароль должен содержать минимум 6 символов",
		"Field validation for 'Password' failed on the 'max' tag":      "Пароль должен содержать максимум 100 символов",

		// RoleID валидация
		"Field validation for 'RoleID' failed on the 'required' tag": "Поле Роль обязательно для заполнения",
		"Field validation for 'RoleID' failed on the 'min' tag":      "Неверный ID роли",

		// FirstName/LastName валидация
		"Field validation for 'FirstName' failed on the 'max' tag": "Имя должно содержать максимум 50 символов",
		"Field validation for 'LastName' failed on the 'max' tag":  "Фамилия должна содержать максимум 50 символов",

		// Phone валидация
		"Field validation for 'Phone' failed on the 'max' tag": "Телефон должен содержать максимум 50 символов",

		// TelegramID валидация
		"Field validation for 'TelegramID' failed on the 'max' tag": "Telegram ID должен содержать максимум 50 символов",

		// UserType валидация
		"Field validation for 'UserType' failed on the 'max' tag": "Тип пользователя должен содержать максимум 50 символов",
	}

	// Ищем точное совпадение
	if translated, exists := translations[errorMsg]; exists {
		return translated
	}

	// Если точного совпадения нет, делаем базовый перевод
	if strings.Contains(errorMsg, "required") {
		return "Все обязательные поля должны быть заполнены"
	}
	if strings.Contains(errorMsg, "email") {
		return "Некорректный формат email адреса"
	}
	if strings.Contains(errorMsg, "min") {
		return "Значение слишком короткое"
	}
	if strings.Contains(errorMsg, "max") {
		return "Значение слишком длинное"
	}

	// Если ничего не подошло, возвращаем общее сообщение
	return "Неверные данные запроса"
}
