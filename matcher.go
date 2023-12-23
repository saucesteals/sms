package sms

import (
	"context"
	"fmt"
	"time"
)

type MatcherFn func(message string) (match string)

type Matcher struct {
	MatcherFn MatcherFn
	Delay     time.Duration
	Timeout   time.Duration
}

func NewMatcher(matcher MatcherFn, delay time.Duration, timeout time.Duration) *Matcher {
	return &Matcher{MatcherFn: matcher, Delay: delay, Timeout: timeout}
}

func (m *Matcher) getMatch(ctx context.Context, client Client, phoneNumber *PhoneNumber) (string, error) {
	messages, err := client.GetMessages(ctx, phoneNumber)
	if err != nil {
		return "", err
	}

	for _, sms := range messages {
		if match := m.MatcherFn(sms); match != "" {
			return match, nil
		}
	}

	return "", nil
}

func (m *Matcher) WaitForMessage(ctx context.Context, client Client, phoneNumber *PhoneNumber) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()

	if match, err := m.getMatch(ctx, client, phoneNumber); err != nil || match != "" {
		return match, err
	}

	ticker := time.NewTicker(m.Delay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("sms: waiting for messages: %w", ctx.Err())
		case <-ticker.C:
			if match, err := m.getMatch(ctx, client, phoneNumber); err != nil || match != "" {
				return match, err
			}
		}
	}
}
