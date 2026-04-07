package redact

type Result struct {
	Value    string
	Redacted bool
}

type Redactor interface {
	Redact(key string, value string) Result
}

type NoopRedactor struct{}

func (NoopRedactor) Redact(_ string, value string) Result {
	return Result{
		Value:    value,
		Redacted: false,
	}
}
