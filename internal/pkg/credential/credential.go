package credential

import (
	"net/http"
)

type Fetcher interface {
	Authorizer(r *http.Request) error
}
