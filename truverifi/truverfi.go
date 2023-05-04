package truverifi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

type changeServicePayload struct {
	Services []string `json:"services"`
}

type changeServiceResponse struct {
	Error       string `json:"error"`
	PhoneNumber string `json:"phoneNumber"`
}

func (c *Client) do(
	ctx context.Context,
	method string,
	path string,
	payload any,
	response any,
) error {
	var bodyReader io.Reader

	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		method,
		"https://app.truverifi.com/api/"+path,
		bodyReader,
	)
	if err != nil {
		return err
	}

	req.Header = http.Header{
		"content-type": []string{"application/json"},
		"x-api-key":    []string{c.apiKey},
	}

	var resp *http.Response
	maxRetries := 3
	retryDelay := 20 * time.Second

	for i := 0; i < maxRetries; i++ {
		resp, err = c.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 429 {
			break
		}

		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
				continue
			}
		}
	}
	if response == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return err
	}

	return nil
}

func (c *Client) GetPhoneNumber(
	ctx context.Context,
	service string,
	_ string,
) (*sms.PhoneNumber, error) {
	var resp changeServiceResponse
	err := c.do(
		ctx,
		http.MethodPost,
		"line/changeService",
		changeServicePayload{Services: []string{service}},
		&resp,
	)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("truverifi: %s", resp.Error)
	}

	number, err := phonenumbers.Parse(resp.PhoneNumber, "US")
	if err != nil {
		return nil, fmt.Errorf("truverifi: parsing phone number (%s): %w", resp.PhoneNumber, err)
	}

	return &sms.PhoneNumber{PhoneNumber: number, Metadata: nil}, nil
}

type lineResponse struct {
	PhoneNumber     string    `json:"phoneNumber"`
	Status          string    `json:"status"`
	ExpirationTime  time.Time `json:"expirationTime"`
	CurrentServices []string  `json:"currentServices"`
	Sms             []lineSms `json:"sms"`
}

type lineSms struct {
	ID          int       `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	PhoneNumber string    `json:"phoneNumber"`
	Text        string    `json:"text"`
}

func (c *Client) GetMessages(ctx context.Context, phoneNumber *sms.PhoneNumber) ([]string, error) {
	resp := &lineResponse{}
	if err := c.do(ctx, http.MethodGet, "line", nil, resp); err != nil {
		return nil, err
	}
	messages := make([]string, len(resp.Sms))
	for i, sms := range resp.Sms {
		messages[i] = sms.Text
	}

	if len(messages) > 1 {
		phoneNumber.MarkUsed()
	}

	return messages, nil
}

func (c *Client) CancelPhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) error {
	// truverifi does not support cancelling
	return nil
}

func (c *Client) ReportPhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) error {
	// truverifi does not support reporting
	return nil
}
