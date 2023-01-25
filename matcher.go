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

func (m *Matcher) WaitForMessage(ctx context.Context, client Client, phoneNumber *PhoneNumber) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()

	cancelPhoneNumber := func() {
		// new ctx - existing could already be cancelled
		_ctx, _cancel := context.WithTimeout(context.Background(), time.Second*5)
		client.CancelPhoneNumber(_ctx, phoneNumber) // ignore err
		_cancel()
	}

	for {
		select {
		case <-ctx.Done():
			cancelPhoneNumber()
			return "", fmt.Errorf("sms: waiting for messages: %w", ctx.Err())
		default:
			messages, err := client.GetMessages(ctx, phoneNumber)
			if err != nil {
				cancelPhoneNumber()
				return "", err
			}

			for _, sms := range messages {
				if match := m.MatcherFn(sms); match != "" {
					return match, nil
				}
			}

			time.Sleep(m.Delay)
		}
	}
}
