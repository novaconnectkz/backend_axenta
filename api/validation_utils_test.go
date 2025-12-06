package api

import (
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTranslateValidationError_Email тестирует перевод ошибок валидации Email
func TestTranslateValidationError_Email(t *testing.T) {
	// Тест для email валидации
	err := errors.New("Field validation for 'Email' failed on the 'email' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Поле Email должно содержать корректный email адрес", result)

	err = errors.New("Field validation for 'Email' failed on the 'required' tag")
	result = translateValidationError(err)
	assert.Equal(t, "Поле Email обязательно для заполнения", result)
}

// TestTranslateValidationError_Username тестирует перевод ошибок валидации Username
func TestTranslateValidationError_Username(t *testing.T) {
	err := errors.New("Field validation for 'Username' failed on the 'required' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Поле Имя пользователя обязательно для заполнения", result)

	err = errors.New("Field validation for 'Username' failed on the 'min' tag")
	result = translateValidationError(err)
	assert.Equal(t, "Имя пользователя должно содержать минимум 3 символа", result)

	err = errors.New("Field validation for 'Username' failed on the 'max' tag")
	result = translateValidationError(err)
	assert.Equal(t, "Имя пользователя должно содержать максимум 50 символов", result)
}

// TestTranslateValidationError_Password тестирует перевод ошибок валидации Password
func TestTranslateValidationError_Password(t *testing.T) {
	err := errors.New("Field validation for 'Password' failed on the 'required' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Поле Пароль обязательно для заполнения", result)

	err = errors.New("Field validation for 'Password' failed on the 'min' tag")
	result = translateValidationError(err)
	assert.Equal(t, "Пароль должен содержать минимум 6 символов", result)

	err = errors.New("Field validation for 'Password' failed on the 'max' tag")
	result = translateValidationError(err)
	assert.Equal(t, "Пароль должен содержать максимум 100 символов", result)
}

// TestTranslateValidationError_Name тестирует перевод ошибок валидации Name
func TestTranslateValidationError_Name(t *testing.T) {
	err := errors.New("Field validation for 'Name' failed on the 'required' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Поле Имя обязательно для заполнения", result)
}

// TestTranslateValidationError_RoleID тестирует перевод ошибок валидации RoleID
func TestTranslateValidationError_RoleID(t *testing.T) {
	err := errors.New("Field validation for 'RoleID' failed on the 'required' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Поле Роль обязательно для заполнения", result)

	err = errors.New("Field validation for 'RoleID' failed on the 'min' tag")
	result = translateValidationError(err)
	assert.Equal(t, "Неверный ID роли", result)
}

// TestTranslateValidationError_FirstName тестирует перевод ошибок валидации FirstName
func TestTranslateValidationError_FirstName(t *testing.T) {
	err := errors.New("Field validation for 'FirstName' failed on the 'max' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Имя должно содержать максимум 50 символов", result)
}

// TestTranslateValidationError_LastName тестирует перевод ошибок валидации LastName
func TestTranslateValidationError_LastName(t *testing.T) {
	err := errors.New("Field validation for 'LastName' failed on the 'max' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Фамилия должна содержать максимум 50 символов", result)
}

// TestTranslateValidationError_Phone тестирует перевод ошибок валидации Phone
func TestTranslateValidationError_Phone(t *testing.T) {
	err := errors.New("Field validation for 'Phone' failed on the 'max' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Телефон должен содержать максимум 50 символов", result)
}

// TestTranslateValidationError_TelegramID тестирует перевод ошибок валидации TelegramID
func TestTranslateValidationError_TelegramID(t *testing.T) {
	err := errors.New("Field validation for 'TelegramID' failed on the 'max' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Telegram ID должен содержать максимум 50 символов", result)
}

// TestTranslateValidationError_UserType тестирует перевод ошибок валидации UserType
func TestTranslateValidationError_UserType(t *testing.T) {
	err := errors.New("Field validation for 'UserType' failed on the 'max' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Тип пользователя должен содержать максимум 50 символов", result)
}

// TestTranslateValidationError_GenericRequired тестирует общий перевод для required
func TestTranslateValidationError_GenericRequired(t *testing.T) {
	err := errors.New("Some field validation failed on the 'required' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Все обязательные поля должны быть заполнены", result)
}

// TestTranslateValidationError_GenericEmail тестирует общий перевод для email
func TestTranslateValidationError_GenericEmail(t *testing.T) {
	err := errors.New("Some field contains invalid email format")
	result := translateValidationError(err)
	assert.Equal(t, "Некорректный формат email адреса", result)
}

// TestTranslateValidationError_GenericMin тестирует общий перевод для min
func TestTranslateValidationError_GenericMin(t *testing.T) {
	err := errors.New("Some field validation failed on the 'min' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Значение слишком короткое", result)
}

// TestTranslateValidationError_GenericMax тестирует общий перевод для max
func TestTranslateValidationError_GenericMax(t *testing.T) {
	err := errors.New("Some field validation failed on the 'max' tag")
	result := translateValidationError(err)
	assert.Equal(t, "Значение слишком длинное", result)
}

// TestTranslateValidationError_UnknownError тестирует обработку неизвестной ошибки
func TestTranslateValidationError_UnknownError(t *testing.T) {
	err := errors.New("Some unknown validation error")
	result := translateValidationError(err)
	assert.Equal(t, "Неверные данные запроса", result)
}

// TestTranslateValidationError_ValidatorError тестирует с реальным validator.Error
func TestTranslateValidationError_ValidatorError(t *testing.T) {
	validate := validator.New()
	type TestStruct struct {
		Email string `validate:"required,email"`
	}

	test := TestStruct{}
	err := validate.Struct(test)
	require.Error(t, err)

	// Преобразуем ошибку валидатора в строку
	errStr := err.Error()
	result := translateValidationError(errors.New(errStr))

	// Проверяем, что результат содержит перевод
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Email")
}
