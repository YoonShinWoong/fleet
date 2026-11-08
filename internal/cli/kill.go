package cli

import (
	"errors"
	"net/url"
)

func runKill(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: fleet kill <name>")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	return c.do("DELETE", "/v1/terms/"+url.PathEscape(args[0]), nil, nil)
}
