package entity

// EventType представляет тип телеком события
type EventType string

const (
	// EventTypeCallOut - исходящий звонок
	EventTypeCallOut EventType = "CALL_OUT"

	// EventTypeCallIn - входящий звонок
	EventTypeCallIn EventType = "CALL_IN"

	// EventTypeSMSSent - отправленное SMS
	EventTypeSMSSent EventType = "SMS_SENT"

	// EventTypeSMSReceived - полученное SMS
	EventTypeSMSReceived EventType = "SMS_RECEIVED"

	// EventTypeData - интернет-сессия
	EventTypeData EventType = "DATA"
)

// IsValid проверяет валидность типа события
func (t EventType) IsValid() bool {
	switch t {
	case EventTypeCallOut, EventTypeCallIn, EventTypeSMSSent, EventTypeSMSReceived, EventTypeData:
		return true
	default:
		return false
	}
}

// String возвращает строковое представление типа события
func (t EventType) String() string {
	return string(t)
}
