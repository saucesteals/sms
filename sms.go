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

	used      bool
	cancelled bool
}

func (p *PhoneNumber) MarkUsed() {
	p.used = true
}

func (p *PhoneNumber) Used() bool {
	return p.used
}

func (p *PhoneNumber) MarkCancelled() {
	p.cancelled = true
}

func (p *PhoneNumber) Cancelled() bool {
	return p.cancelled
}

func (p *PhoneNumber) Format(format phonenumbers.PhoneNumberFormat) string {
	return phonenumbers.Format(p.PhoneNumber, format)
}

type Client interface {
	GetPhoneNumber(ctx context.Context, service string, country string) (*PhoneNumber, error)
	GetMessages(ctx context.Context, phoneNumber *PhoneNumber) ([]string, error)
	CancelPhoneNumber(ctx context.Context, phoneNumber *PhoneNumber) error
	ReportPhoneNumber(ctx context.Context, phoneNumber *PhoneNumber) error
}
