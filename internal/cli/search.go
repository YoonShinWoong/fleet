package cli

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func runSearch(args []string) error {
	q := strings.TrimSpace(strings.Join(args, " "))
	if len([]rune(q)) < 2 {
		return errors.New("usage: keepty search <text> (2+ characters)")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	var hits []struct {
		Term      string   `json:"term"`
		Workspace string   `json:"workspace"`
		Count     int      `json:"count"`
		Lines     []string `json:"lines"`
	}
	if err := c.do("GET", "/v1/search?q="+url.QueryEscape(q), nil, &hits); err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Println("no matches")
		return nil
	}
	for _, h := range hits {
		ws := ""
		if h.Workspace != "" {
			ws = "  [" + h.Workspace + "]"
		}
		fmt.Printf("%s%s  (%d)\n", h.Term, ws, h.Count)
		for _, l := range h.Lines {
			fmt.Printf("    %s\n", clip(l, 120))
		}
	}
	return nil
}
