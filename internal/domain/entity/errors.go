package entity

import "errors"

var (
	// ErrInvalidEventID возвращается при невалидном event_id
	ErrInvalidEventID = errors.New("invalid event_id")

	// ErrInvalidSourceID возвращается при невалидном source_id
	ErrInvalidSourceID = errors.New("invalid source_id")

	// ErrInvalidUserID возвращается при невалидном user_id
	ErrInvalidUserID = errors.New("invalid user_id")

	// ErrInvalidPhone возвращается при невалидном номере телефона
	ErrInvalidPhone = errors.New("invalid phone number")

	// ErrInvalidEventType возвращается при невалидном типе события
	ErrInvalidEventType = errors.New("invalid event type")

	// ErrInvalidStartedAt возвращается при невалидном времени начала
	ErrInvalidStartedAt = errors.New("invalid started_at time")

	// ErrEndedBeforeStarted возвращается когда ended_at < started_at
	ErrEndedBeforeStarted = errors.New("ended_at is before started_at")

	// ErrInvalidTimeRange возвращается при невалидном временном диапазоне
	ErrInvalidTimeRange = errors.New("invalid time range")

	// ErrTimeRangeTooLarge возвращается когда запрошен слишком большой период
	ErrTimeRangeTooLarge = errors.New("time range is too large")

	// ErrEventNotFound возвращается когда событие не найдено
	ErrEventNotFound = errors.New("event not found")

	// ErrDuplicateEvent возвращается при попытке вставить дубликат события
	ErrDuplicateEvent = errors.New("duplicate event")
)
