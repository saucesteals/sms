package smspool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/nyaruka/phonenumbers"
	"github.com/saucesteals/sms"
)

var (
	ErrVerificationExpired = errors.New("smspool: verification expired")
	ErrReported            = errors.New("smspool: verification reported")
	ErrCancelled           = errors.New("smspool: verification was cancelled by user or system")

	ErrUnauthorized = errors.New("smspool: unauthorized")
)

type Client struct {
	sms.Client

	http   *http.Client
	apiKey string
}

type metadata struct {
	id string
}

func NewClient(apiKey string) *Client {
	return &Client{
		http:   http.DefaultClient,
		apiKey: apiKey,
	}
}

func (c *Client) do(ctx context.Context, method string, path string, query url.Values, response any) error {
	if query == nil {
		query = url.Values{}
	}

	query.Set("key", c.apiKey)

	url := "https://api.smspool.net/" + path + "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		if resp.StatusCode == http.StatusUnauthorized {
			return ErrUnauthorized
		}
		return fmt.Errorf("smspool: %d %s", resp.StatusCode, resp.Status)
	}

	if response == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return err
	}

	return nil
}

type verification struct {
	Success     int    `json:"success"`
	Number      string `json:"number"`
	Cc          string `json:"cc"`
	Phonenumber string `json:"phonenumber"`
	OrderID     string `json:"order_id"`
	Country     string `json:"country"`
	Service     string `json:"service"`
	Pool        int    `json:"pool"`
	ExpiresIn   int    `json:"expires_in"`
	Message     string `json:"message"`
	Cost        string `json:"cost"`
}

type smscheck struct {
	Success    int    `json:"success"`
	Message    string `json:"message"`
	Status     int    `json:"status"`
	Sms        string `json:"sms"`
	FullSms    string `json:"full_sms"`
	Expiration int    `json:"expiration"`
}

type service struct {
	Id   int    `json:"ID"`
	Name string `json:"name"`
}

func (c *Client) GetServices(ctx context.Context) ([]service, error) {
	var services []service
	err := c.do(ctx, http.MethodGet, "service/retrieve_all", nil, &services)
	if err != nil {
		return nil, err
	}

	return services, nil
}

func (c *Client) GetPhoneNumber(ctx context.Context, serviceId string, _ string) (*sms.PhoneNumber, error) {
	var resp verification
	err := c.do(ctx, http.MethodGet, "purchase/sms", url.Values{
		"country": {"US"},
		"service": {serviceId},
	}, &resp)
	if err != nil {
		return nil, err
	}

	number, err := phonenumbers.Parse(resp.Number, "US")
	if err != nil {
		return nil, fmt.Errorf("smspool: parsing phone number (%s): %w", resp.Number, err)
	}

	return &sms.PhoneNumber{PhoneNumber: number, Metadata: metadata{id: resp.OrderID}}, nil
}

func (c *Client) GetMessages(ctx context.Context, phoneNumber *sms.PhoneNumber) ([]string, error) {
	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}

	resp := &smscheck{}
	if err := c.do(ctx, http.MethodGet, "sms/check", url.Values{
		"orderid": {metadata.id},
	}, resp); err != nil {
		return nil, err
	}

	if resp.Success == 0 {
		return nil, fmt.Errorf("smspool: %s", resp.Message)
	}

	switch resp.Status {
	case 1:
		return []string{}, nil
	case 2:
		return nil, ErrVerificationExpired
	case 3:
		phoneNumber.MarkUsed()

		return []string{resp.FullSms}, nil
	case 4:
		return nil, ErrReported
	case 5:
		return nil, ErrCancelled
	default:
		return nil, fmt.Errorf("smspool: unknown status %d", resp.Status)
	}
}

func (c *Client) CancelPhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) error {
	if phoneNumber.Used() || phoneNumber.Cancelled() {
		return nil
	}

	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return sms.ErrInvalidMetadata
	}

	resp := &smscheck{}
	err := c.do(ctx, http.MethodGet, "sms/cancel", url.Values{
		"orderID": {metadata.id},
	}, resp)
	if err != nil {
		return err
	}

	if resp.Success == 0 {
		return fmt.Errorf("smspool: %s", resp.Message)
	}

	phoneNumber.MarkCancelled()
	return nil
}
