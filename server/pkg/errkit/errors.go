package errkit

import "fmt"

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func Wrap(code string, err error) error {
	if err == nil {
		return nil
	}
	return Error{Code: code, Message: err.Error()}
}
