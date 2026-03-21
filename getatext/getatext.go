package getatext

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/nyaruka/phonenumbers"
	"github.com/saucesteals/sms"
)

const baseURL = "https://getatext.com/api/v1"

type Client struct {
	http   *http.Client
	apiKey string
}

var _ sms.ReusableClient = &Client{}

type metadata struct {
	id             int
	lastCode       string
	ignoreLastCode bool
}


func NewClient(apiKey string) *Client {
	return &Client{
		http:   http.DefaultClient,
		apiKey: apiKey,
	}
}

type errorResponse struct {
	Errors string `json:"errors"`
}

func (c *Client) do(ctx context.Context, method string, path string, payload any, response any) error {
	var bodyReader io.Reader

	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Auth", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		if resp.StatusCode == http.StatusTooManyRequests {
			return sms.ErrRatelimited
		}
		var errResp errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Errors != "" {
			return fmt.Errorf("getatext: %s", errResp.Errors)
		}
		return fmt.Errorf("getatext: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	if response == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(response)
}

type rentRequest struct {
	Service string `json:"service"`
}

type rentResponse struct {
	ID          int            `json:"id"`
	Status      string         `json:"status"`
	Number      string         `json:"number"`
	ServiceName string         `json:"service_name"`
	Price       json.Number    `json:"price"`
	NewBalance  json.Number    `json:"new_balance"`
	EndTime     string         `json:"end_time"`
}

func (c *Client) GetPhoneNumber(ctx context.Context, service string, _ string) (*sms.PhoneNumber, error) {
	req := rentRequest{Service: service}

	var resp rentResponse
	if err := c.do(ctx, http.MethodPost, "/rent-a-number", req, &resp); err != nil {
		return nil, err
	}

	number, err := phonenumbers.Parse("+1"+resp.Number, "US")
	if err != nil {
		return nil, fmt.Errorf("getatext: parsing phone number (%s): %w", resp.Number, err)
	}

	return &sms.PhoneNumber{PhoneNumber: number, Metadata: metadata{id: resp.ID}}, nil
}

type statusRequest struct {
	ID int `json:"id"`
}

type statusResponse struct {
	ID          int     `json:"id"`
	Status      string  `json:"status"`
	Code        *string `json:"code"`
	Number      string  `json:"number"`
	ServiceName string  `json:"service_name"`
	Cost        string  `json:"cost"`
}

func (c *Client) GetMessages(ctx context.Context, phoneNumber *sms.PhoneNumber) ([]string, error) {
	meta, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}

	var resp statusResponse
	if err := c.do(ctx, http.MethodPost, "/rental-status", statusRequest{ID: meta.id}, &resp); err != nil {
		return nil, err
	}

	if resp.Code == nil {
		return []string{}, nil
	}

	code := *resp.Code
	if meta.ignoreLastCode && meta.lastCode == code {
		return []string{}, nil
	}

	meta.lastCode = code
	phoneNumber.Metadata = meta

	phoneNumber.MarkUsed()
	return []string{code}, nil
}

type cancelRequest struct {
	ID int `json:"id"`
}

func (c *Client) CancelPhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) error {
	if phoneNumber.Used() || phoneNumber.Cancelled() {
		return nil
	}

	meta, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return sms.ErrInvalidMetadata
	}

	if err := c.do(ctx, http.MethodPost, "/cancel-rental", cancelRequest{ID: meta.id}, nil); err != nil {
		return err
	}

	phoneNumber.MarkCancelled()
	return nil
}

func (c *Client) ReportPhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) error {
	return c.CancelPhoneNumber(ctx, phoneNumber)
}

func (c *Client) ReusePhoneNumber(ctx context.Context, phoneNumber *sms.PhoneNumber) (*sms.PhoneNumber, error) {
	meta, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}

	meta.ignoreLastCode = true
	phoneNumber.Metadata = meta
	phoneNumber.Reuse()
	return phoneNumber, nil
}

type balanceResponse struct {
	Status  string `json:"status"`
	Balance string `json:"balance"`
}

func (c *Client) GetBalance(ctx context.Context) (float64, error) {
	var resp balanceResponse
	if err := c.do(ctx, http.MethodGet, "/balance", nil, &resp); err != nil {
		return 0, err
	}

	bal, err := strconv.ParseFloat(resp.Balance, 64)
	if err != nil {
		return 0, fmt.Errorf("getatext: parsing balance %q: %w", resp.Balance, err)
	}

	return bal, nil
}

type Service struct {
	ServiceName string  `json:"service_name"`
	APIName     string  `json:"api_name"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	TTL         int     `json:"ttl"`
}

type pricesResponse struct {
	Prices []Service `json:"prices"`
}

func (c *Client) GetServices(ctx context.Context) ([]Service, error) {
	var resp pricesResponse
	if err := c.do(ctx, http.MethodGet, "/prices-info", nil, &resp); err != nil {
		return nil, err
	}

	return resp.Prices, nil
}
