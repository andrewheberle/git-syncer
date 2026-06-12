package credential

import (
	"fmt"
)

type AuthType struct {
	v string
}

const (
	BasicAuth  = "basic"
	BearerAuth = "bearer"
)

func (a AuthType) String() string {
	if a.v == "" || a.v == BasicAuth {
		return BasicAuth
	}

	return BearerAuth
}

func (a AuthType) Type() string {
	return "string (basic or bearer)"
}

func (a *AuthType) Set(s string) error {
	switch s {
	case BasicAuth:
		a.v = BasicAuth
	case BearerAuth:
		a.v = BearerAuth
	default:
		return fmt.Errorf("unsupported authtype")
	}

	return nil
}
