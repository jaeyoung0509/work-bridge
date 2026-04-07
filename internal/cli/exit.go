package cli

const (
	ExitOK             = 0
	ExitRuntime        = 1
	ExitUsage          = 2
	ExitNotImplemented = 10
)

type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string {
	return e.Message
}

func newExitError(code int, message string) error {
	return &ExitError{
		Code:    code,
		Message: message,
	}
}
