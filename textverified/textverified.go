package textverified

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/nyaruka/phonenumbers"
	"github.com/saucesteals/sms"
)

var (
	ErrVerificationExpired = errors.New("textverified: verification expired")
	ErrReported            = errors.New("textverified: verification reported")
	ErrCancelled           = errors.New("textverified: verification was cancelled by user or system")

	ErrUnauthorized = errors.New("textverified: unauthorized")
)

type Client struct {
	sms.Client

	http   *http.Client
	apiKey string

	authDetails *AuthDetails
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

func statusText(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "Create failure"
	case http.StatusPaymentRequired:
		return "Insufficient credits"
	case http.StatusTooManyRequests:
		return "Too many pending verifications. Complete the pending verifications before creating additional ones."
	default:
		return http.StatusText(code)
	}
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

	req, err := http.NewRequestWithContext(ctx, method, "https://www.textverified.com/api/"+path, bodyReader)
	if err != nil {
		return err
	}

	req.Header = http.Header{
		"content-type":              []string{"application/json"},
		"x-simple-api-access-token": []string{c.apiKey},
	}

	if c.authDetails != nil {
		req.Header.Add("authorization", "Bearer "+c.authDetails.BearerToken)
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
		return fmt.Errorf("textverified: %d %s", resp.StatusCode, statusText(resp.StatusCode))
	}

	if response == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return err
	}

	return nil
}

type AuthDetails struct {
	BearerToken string    `json:"bearer_token"`
	Expiration  time.Time `json:"expiration"`
	Ticks       int64     `json:"ticks"`
}

func (c *Client) Authenticate(ctx context.Context) error {
	var resp AuthDetails
	err := c.do(ctx, http.MethodPost, "SimpleAuthentication", nil, &resp)
	if err != nil {
		return err
	}

	c.authDetails = &resp
	return nil
}

func (c *Client) KeepAuthAlive(ctx context.Context) error {
	var initialDuration time.Duration
	if c.authDetails != nil {
		initialDuration = time.Until(c.authDetails.Expiration) - time.Minute
	}
	timer := time.NewTimer(initialDuration)

	for {
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
			for {
				if err := c.Authenticate(ctx); err != nil {
					if errors.Is(err, ErrUnauthorized) {
						return err
					}
					// retry every 5 seconds otherwise
					time.Sleep(time.Second * 5)
					continue
				}
				timer.Reset(time.Until(c.authDetails.Expiration) - time.Minute)
				break
			}
		}
	}

}

type createVerificationRequest struct {
	ID int64 `json:"id"`
}

type createVerificationResponse struct {
	ID              string  `json:"id"`
	Cost            float64 `json:"cost"`
	TargetName      string  `json:"target_name"`
	Number          string  `json:"number"`
	SenderNumber    string  `json:"sender_number"`
	TimeRemaining   string  `json:"time_remaining"`
	ReuseWindow     string  `json:"reuse_window"`
	Status          string  `json:"status"`
	Sms             string  `json:"sms"`
	Code            string  `json:"code"`
	VerificationURI string  `json:"verification_uri"`
	CancelURI       string  `json:"cancel_uri"`
	ReportURI       string  `json:"report_uri"`
	ReuseURI        string  `json:"reuse_uri"`
}

func (c *Client) GetPhoneNumber(ctx context.Context, serviceId string, _ string) (*sms.PhoneNumber, error) {
	id, err := strconv.ParseInt(serviceId, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("textverified: invalid service id: %w", err)
	}

	var resp createVerificationResponse
	err = c.do(ctx, http.MethodPost, "Verifications", createVerificationRequest{ID: id}, &resp)
	if err != nil {
		return nil, err
	}

	number, err := phonenumbers.Parse(resp.Number, "US")
	if err != nil {
		return nil, fmt.Errorf("textverified: parsing phone number (%s): %w", resp.Number, err)
	}

	return &sms.PhoneNumber{PhoneNumber: number, Metadata: metadata{id: resp.ID}}, nil
}

type verificationDetailsResponse struct {
	ID              string  `json:"id"`
	Cost            float64 `json:"cost"`
	TargetName      string  `json:"target_name"`
	Number          string  `json:"number"`
	SenderNumber    string  `json:"sender_number"`
	TimeRemaining   string  `json:"time_remaining"`
	ReuseWindow     string  `json:"reuse_window"`
	Status          string  `json:"status"`
	Sms             string  `json:"sms"`
	Code            string  `json:"code"`
	VerificationURI string  `json:"verification_uri"`
	CancelURI       string  `json:"cancel_uri"`
	ReportURI       string  `json:"report_uri"`
	ReuseURI        string  `json:"reuse_uri"`
}

func (c *Client) GetMessages(ctx context.Context, phoneNumber *sms.PhoneNumber) ([]string, error) {
	metadata, ok := phoneNumber.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}

	resp := &verificationDetailsResponse{}
	if err := c.do(ctx, http.MethodGet, "Verifications/"+metadata.id, nil, resp); err != nil {
		return nil, err
	}

	switch resp.Status {
	case "Pending":
		// sms: null (no messages yet)
		return []string{}, nil
	case "Timed Out":
		return nil, ErrVerificationExpired
	case "Reported":
		return nil, ErrReported
	case "Cancelled":
		return nil, ErrCancelled
	}

	return []string{resp.Sms}, nil
}
