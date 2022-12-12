package sms

import (
	"context"
	"errors"

	"github.com/nyaruka/phonenumbers"
)

var (
	ErrInvalidMetadata = errors.New("sms: invalid metadata type")
)

type PhoneNumber struct {
	*phonenumbers.PhoneNumber
	Metadata any
}

func (p *PhoneNumber) Format(format phonenumbers.PhoneNumberFormat) string {
	return phonenumbers.Format(p.PhoneNumber, format)
}

type Client interface {
	GetPhoneNumber(ctx context.Context, service string, country string) (*PhoneNumber, error)
	GetMessages(ctx context.Context, metadata *PhoneNumber) ([]string, error)
}
