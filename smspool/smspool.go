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

type apiResponse struct {
	Success int    `json:"success"`
	Message string `json:"message"`
}

type verification struct {
	apiResponse
	Number      int    `json:"number"`
	CC          string `json:"cc"`
	Phonenumber string `json:"phonenumber"`
	OrderID     string `json:"order_id"`
	Country     string `json:"country"`
	Service     string `json:"service"`
	Pool        int    `json:"pool"`
	ExpiresIn   int    `json:"expires_in"`
	Cost        string `json:"cost"`
}

type smsCheckResponse struct {
	apiResponse
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

func (c *Client) GetPhoneNumber(ctx context.Context, serviceId string, country string) (*sms.PhoneNumber, error) {
	var res verification
	err := c.do(ctx, http.MethodGet, "purchase/sms", url.Values{
		"country": {country},
		"service": {serviceId},
	}, &res)
	if err != nil {
		return nil, err
	}

	if res.Success == 0 {
		return nil, fmt.Errorf("smspool: %s", res.Message)
	}

	number, err := phonenumbers.Parse(fmt.Sprintf("+%s%s", res.CC, res.Phonenumber), "US")
	if err != nil {
		return nil, fmt.Errorf("smspool: parsing phone number (%s): %w", res.Phonenumber, err)
	}

	return &sms.PhoneNumber{PhoneNumber: number, Metadata: metadata{id: res.OrderID}}, nil
}

func (c *Client) GetMessages(ctx context.Context, phoneNumber *sms.PhoneNumber) ([]string, error) {
	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}

	var res smsCheckResponse
	if err := c.do(ctx, http.MethodGet, "sms/check", url.Values{
		"orderid": {metadata.id},
	}, &res); err != nil {
		return nil, err
	}

	switch res.Status {
	case 1, 4:
		return []string{}, nil
	case 2:
		return nil, ErrVerificationExpired
	case 3:
		phoneNumber.MarkUsed()

		return []string{res.FullSms}, nil
	case 5:
		return nil, ErrCancelled
	default:
		return nil, fmt.Errorf("smspool: unknown status %d", res.Status)
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

	var res apiResponse
	err := c.do(ctx, http.MethodGet, "sms/cancel", url.Values{
		"orderid": {metadata.id},
	}, &res)
	if err != nil {
		return err
	}

	if res.Success == 0 {
		return fmt.Errorf("smspool: %s", res.Message)
	}

	phoneNumber.MarkCancelled()
	return nil
}

func (c *Client) ReusePhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) (*sms.PhoneNumber, error) {
	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}

	var res apiResponse
	err := c.do(ctx, http.MethodPost, "sms/resend", url.Values{
		"orderid": {metadata.id},
	}, &res)
	if err != nil {
		return nil, err
	}

	if res.Success == 0 {
		return nil, fmt.Errorf("smspool: %s", res.Message)
	}

	return phoneNumber, nil
}
