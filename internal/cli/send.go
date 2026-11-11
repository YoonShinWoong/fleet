package cli

import (
	"errors"
	"net/url"
	"strings"
)

func runSend(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: keepty send <name> <text...>")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	body := map[string]any{"text": strings.Join(args[1:], " "), "enter": true}
	return c.do("POST", "/v1/terms/"+url.PathEscape(args[0])+"/send", body, nil)
}
