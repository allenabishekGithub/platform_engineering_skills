package server

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type failKind string

const (
	failNone          failKind = ""
	failRetryable     failKind = "retryable"
	failNonRetryable  failKind = "nonretryable"
)

type Behavior struct {
	Raw             string
	Delay           time.Duration
	FlakyCount      int
	Fail            failKind
	Hang            bool
	SucceedThenHang bool
}

func ParseBehavior(raw string) (Behavior, error) {
	b := Behavior{Raw: raw}
	tokens := strings.Split(strings.TrimSpace(raw), ",")
	if raw == "" {
		return b, nil
	}
	seen := map[string]bool{}
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			return b, errors.New("empty behavior token")
		}
		switch {
		case tok == "ok":
		case tok == "hang":
			b.Hang = true
			seen["hang"] = true
		case tok == "succeed-then-hang":
			b.SucceedThenHang = true
		case tok == "fail-retryable":
			b.Fail = failRetryable
		case tok == "fail-nonretryable":
			b.Fail = failNonRetryable
		case strings.HasPrefix(tok, "flaky:"):
			n, err := strconv.Atoi(strings.TrimPrefix(tok, "flaky:"))
			if err != nil || n < 0 {
				return b, fmt.Errorf("bad flaky count %q", tok)
			}
			b.FlakyCount = n
		case strings.HasPrefix(tok, "delay:"):
			d, err := time.ParseDuration(strings.TrimPrefix(tok, "delay:"))
			if err != nil || d < 0 {
				return b, fmt.Errorf("bad delay %q", tok)
			}
			b.Delay = d
		default:
			return b, fmt.Errorf("unknown behavior token %q", tok)
		}
	}
	if b.Hang && b.SucceedThenHang {
		return b, errors.New("hang and succeed-then-hang are mutually exclusive")
	}
	return b, nil
}

func (b Behavior) IsValid() bool {
	_, err := ParseBehavior(b.Raw)
	return err == nil
}
