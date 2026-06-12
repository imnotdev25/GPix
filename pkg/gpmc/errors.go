package gpmc

import (
	"errors"
	"fmt"
)

var ErrUploadRejected = errors.New("gpmc: commit returned no media key")

type AuthError struct {
	Status int
	Body   string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("gpmc auth: status=%d body=%q", e.Status, e.Body)
}

type APIError struct {
	Op     string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("gpmc %s: status=%d body=%q", e.Op, e.Status, e.Body)
}
