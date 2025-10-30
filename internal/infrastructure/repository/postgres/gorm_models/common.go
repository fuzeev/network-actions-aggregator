package gorm_models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JSONB кастомный тип для работы с JSONB в PostgreSQL
type JSONB map[string]interface{}

// Value реализует driver.Valuer для записи в БД
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan реализует sql.Scanner для чтения из БД
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = make(map[string]interface{})
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	if len(bytes) == 0 {
		*j = make(map[string]interface{})
		return nil
	}

	result := make(map[string]interface{})
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}

	*j = result
	return nil
}
