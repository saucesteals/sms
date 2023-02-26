package smsman

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/nyaruka/phonenumbers"
	"github.com/saucesteals/sms"
)

type Client struct {
	sms.Client

	http   *http.Client
	apiKey string
}

func NewClient(apiKey string) *Client {
	return &Client{
		http:   http.DefaultClient,
		apiKey: apiKey,
	}
}

type metadata struct {
	requestID string
}

type smsManResponse interface {
	Failed() bool
	Error() error
}

type errorResponse struct {
	ErrorCode string `json:"error_code"`
	ErrorMsg  any    `json:"error_msg"`
}

func (e *errorResponse) Failed() bool {
	return e.ErrorCode != "" && e.ErrorCode != "wait_sms"
}

func (e *errorResponse) Error() error {
	return fmt.Errorf("%#v (%s)", e.ErrorMsg, e.ErrorCode)
}

type getPhoneNumberResponse struct {
	errorResponse
	RequestID int    `json:"request_id"`
	Number    string `json:"number"`
}

func (c *Client) do(ctx context.Context, action string, query url.Values, response smsManResponse) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://api.sms-man.com/control/"+action, nil)
	if err != nil {
		return err
	}

	query.Set("token", c.apiKey)
	req.URL.RawQuery = query.Encode()

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if response == nil {
		response = &errorResponse{}
	}

	if err := json.NewDecoder(res.Body).Decode(response); err != nil {
		return fmt.Errorf("smsman: decoding response: %w", err)
	}

	if response.Failed() {
		return response.Error()
	}

	return nil
}

func (c *Client) GetPhoneNumber(ctx context.Context, service string, country string) (*sms.PhoneNumber, error) {
	var data getPhoneNumberResponse
	if err := c.do(ctx, "get-number", url.Values{
		"country_id":     {country},
		"application_id": {service},
	}, &data); err != nil {
		return nil, err
	}

	number, err := phonenumbers.Parse(data.Number, "US")
	if err != nil {
		return nil, fmt.Errorf("smsman: parsing phone number (%s): %w", data.Number, err)
	}

	return &sms.PhoneNumber{
		PhoneNumber: number,
		Metadata:    metadata{requestID: strconv.Itoa(data.RequestID)},
	}, nil
}

type getSmsResponse struct {
	errorResponse
	SmsCode string `json:"sms_code"`
}

func (c *Client) GetMessages(ctx context.Context, phoneNumber *sms.PhoneNumber) ([]string, error) {
	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}
	var data getSmsResponse

	if err := c.do(ctx, "get-sms", url.Values{
		"request_id": {metadata.requestID},
	}, &data); err != nil {
		return nil, err
	}

	if data.SmsCode == "" {
		return []string{}, nil
	}

	phoneNumber.MarkUsed()

	return []string{data.SmsCode}, nil
}

func (c *Client) CancelPhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) error {
	if phoneNumber.Used() || phoneNumber.Cancelled() {
		return nil
	}

	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return sms.ErrInvalidMetadata
	}

	if err := c.do(ctx, "set-status", url.Values{
		"status":     {"reject"},
		"request_id": {metadata.requestID},
	}, nil); err != nil {
		return err
	}

	phoneNumber.MarkCancelled()

	return nil
}

func (c *Client) ReportPhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) error {
	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return sms.ErrInvalidMetadata
	}

	if err := c.do(ctx, "set-status", url.Values{
		"status":     {"ok"},
		"request_id": {metadata.requestID},
	}, nil); err != nil {
		return err
	}

	return nil
}
