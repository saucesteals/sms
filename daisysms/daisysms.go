package daisysms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nyaruka/phonenumbers"
	"github.com/saucesteals/sms"
)

type Client struct {
	http   *http.Client
	apiKey string
}

var (
	_ sms.ReusableClient = &Client{}
)

type metadata struct {
	id             string
	lastCode       string
	ignoreLastCode bool
}

func NewClient(apiKey string) *Client {
	return &Client{
		http:   http.DefaultClient,
		apiKey: apiKey,
	}
}

func (c *Client) do(ctx context.Context, query url.Values) (string, error) {
	if query == nil {
		query = url.Values{}
	}

	query.Set("api_key", c.apiKey)

	url := "https://daisysms.com/stubs/handler_api.php?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (c *Client) GetPhoneNumber(ctx context.Context, service string, _ string) (*sms.PhoneNumber, error) {
	res, err := c.do(ctx, url.Values{
		"action":  {"getNumber"},
		"service": {service},
	})
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(res, "ACCESS_NUMBER") {
		return nil, fmt.Errorf("daisysms: %s", res)
	}

	numCols := 3
	parts := strings.SplitN(res, ":", numCols)
	if len(parts) != numCols {
		return nil, fmt.Errorf("daisysms: invalid phone format %q", res)
	}

	id := parts[1]
	number, err := phonenumbers.Parse("+"+parts[2], "US")
	if err != nil {
		return nil, fmt.Errorf("daisysms: parsing phone number for %q", res)
	}

	return &sms.PhoneNumber{
		PhoneNumber: number,
		Metadata:    metadata{id: id},
	}, nil
}

func (c *Client) CancelPhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) error {
	if phoneNumber.Cancelled() {
		return nil
	}

	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return sms.ErrInvalidMetadata
	}

	var status, success string
	if phoneNumber.Used() {
		status = "6"
		success = "ACCESS_ACTIVATION"
	} else {
		status = "8"
		success = "ACCESS_CANCEL"
	}

	res, err := c.do(ctx, url.Values{
		"action": {"setStatus"},
		"status": {status},
		"id":     {metadata.id},
	})
	if err != nil {
		return err
	}

	if res != success {
		return fmt.Errorf("daisysms: failed to cancel %q", res)
	}

	phoneNumber.MarkCancelled()
	return nil
}

func (c *Client) ReportPhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) error {
	return c.CancelPhoneNumber(ctx, phoneNumber)
}

func (c *Client) ReusePhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) (*sms.PhoneNumber, error) {
	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}

	metadata.ignoreLastCode = true
	phoneNumber.Metadata = metadata
	phoneNumber.Reuse()
	return phoneNumber, nil
}

func (c *Client) GetMessages(ctx context.Context, phoneNumber *sms.PhoneNumber) ([]string, error) {
	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}

	res, err := c.do(ctx, url.Values{
		"action": {"getStatus"},
		"id":     {metadata.id},
	})
	if err != nil {
		return nil, err
	}

	if res == "STATUS_WAIT_CODE" {
		return []string{}, nil
	}

	if !strings.HasPrefix(res, "STATUS_OK") {
		return nil, fmt.Errorf("daisysms: failed to get messages: %q", res)
	}

	numCols := 2
	parts := strings.SplitN(res, ":", numCols)
	if len(parts) != numCols {
		return nil, fmt.Errorf("daisysms: invalid messages %q", res)
	}

	code := parts[1]
	if metadata.ignoreLastCode && metadata.lastCode == code {
		return []string{}, nil
	}

	metadata.lastCode = code
	phoneNumber.Metadata = metadata

	phoneNumber.MarkUsed()
	return []string{code}, nil
}
