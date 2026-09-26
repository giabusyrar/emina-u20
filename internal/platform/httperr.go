package goplatform

import "github.com/labstack/echo/v4"

// ErrorBody is the error envelope of both Go services. It is a superset of
// the two shapes that existed before standardization ({"error"} and
// {"success":false,"error"}), so no existing client loses a field it reads.
//
// The vision controller in core-engine does not use it: there, "error" is a
// machine code, not a message.
type ErrorBody struct {
	Success   bool     `json:"success"`
	Error     string   `json:"error"`
	ErrorCode string   `json:"error_code,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

// validationFailedMessage fills "error" on validation responses, which
// previously carried only "errors".
const validationFailedMessage = "validation failed"

func JSONError(c echo.Context, status int, msg string) error {
	return c.JSON(status, ErrorBody{Error: msg})
}

func JSONErrorCode(c echo.Context, status int, code, msg string) error {
	return c.JSON(status, ErrorBody{Error: msg, ErrorCode: code})
}

func JSONValidationError(c echo.Context, status int, errs []string) error {
	return c.JSON(status, ErrorBody{Error: validationFailedMessage, Errors: errs})
}
