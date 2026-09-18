package errors

import "fmt"

const (
	ErrOrderNotFound           = "ERR_ORDER_NOT_FOUND"
	ErrItemNotFound            = "ERR_ITEM_NOT_FOUND"
	ErrInsufficientCash        = "ERR_INSUFFICIENT_CASH"
	ErrPaymentFailed           = "ERR_PAYMENT_FAILED"
	ErrOrderNotPaid            = "ERR_ORDER_NOT_PAID"
	ErrFinalizeFailed          = "ERR_FINALIZE_FAILED"
	ErrTelemetryForwardFailed  = "ERR_TELEMETRY_FORWARD_FAILED"
	ErrInternalError           = "ERR_INTERNAL_ERROR"
	ErrInvalidRequest          = "ERR_INVALID_REQUEST"
)

type POSError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *POSError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func New(code, message string) *POSError {
	return &POSError{
		Code:    code,
		Message: message,
	}
}
