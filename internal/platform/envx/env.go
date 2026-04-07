package envx

import "os"

type Env interface {
	LookupEnv(key string) (string, bool)
}

type OSEnv struct{}

func (OSEnv) LookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}
