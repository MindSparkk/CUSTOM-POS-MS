package errors

import "fmt"

const (
	CategoryBusiness       = "BUSINESS"
	CategoryInfrastructure = "INFRASTRUCTURE"

	ErrOrderNotFound          = "ERR_ORDER_NOT_FOUND"
	ErrItemNotFound           = "ERR_ITEM_NOT_FOUND"
	ErrInsufficientCash       = "ERR_INSUFFICIENT_CASH"
	ErrPaymentFailed          = "ERR_PAYMENT_FAILED"
	ErrOrderNotPaid           = "ERR_ORDER_NOT_PAID"
	ErrFinalizeFailed         = "ERR_FINALIZE_FAILED"
	ErrTelemetryForwardFailed = "ERR_TELEMETRY_FORWARD_FAILED"
	ErrInternalError          = "ERR_INTERNAL_ERROR"
	ErrInvalidRequest         = "ERR_INVALID_REQUEST"
	ErrDatabaseUnavailable    = "ERR_DATABASE_UNAVAILABLE"
)

type POSError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Category string `json:"category"`
}

func (e *POSError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Category, e.Code, e.Message)
}

func GetCategory(code string) string {
	switch code {
	case ErrOrderNotFound, ErrItemNotFound, ErrInsufficientCash, ErrOrderNotPaid, ErrInvalidRequest:
		return CategoryBusiness
	default:
		return CategoryInfrastructure
	}
}

func New(code, message string) *POSError {
	return &POSError{
		Code:     code,
		Message:  message,
		Category: GetCategory(code),
	}
}
