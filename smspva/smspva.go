package smspva

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
	id      string
	service string
	country string
}

type getPhoneNumberResponse struct {
	Response    string `json:"response"`
	Number      string `json:"number"`
	CountryCode string `json:"CountryCode"`
	ID          int    `json:"id"`
}

func (c *Client) do(ctx context.Context, query url.Values, response any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://smspva.com/priemnik.php", nil)
	if err != nil {
		return err
	}

	query.Set("apikey", c.apiKey)
	req.URL.RawQuery = query.Encode()

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if response != nil {
		if err := json.NewDecoder(res.Body).Decode(response); err != nil {
			return fmt.Errorf("smspva: decoding response: %w", err)
		}
	}

	return nil
}

func (c *Client) GetPhoneNumber(ctx context.Context, service string, country string) (*sms.PhoneNumber, error) {
	var data getPhoneNumberResponse
	c.do(ctx, url.Values{
		"metod":   {"get_number"},
		"country": {country},
		"service": {service},
	}, &data)

	if data.Response != "1" {
		return nil, fmt.Errorf("smspva: get_number bad response %+v", data)
	}

	number, err := phonenumbers.Parse(data.CountryCode+data.Number, "")
	if err != nil {
		return nil, fmt.Errorf("smspva: parsing phone number (%s %s): %w", data.CountryCode, data.Number, err)
	}

	return &sms.PhoneNumber{
		PhoneNumber: number,
		Metadata:    metadata{id: strconv.Itoa(data.ID), service: service, country: country},
	}, nil
}

type getMessagesResponse struct {
	Response string `json:"response"`
	Number   string `json:"number"`
	Text     string `json:"text"`
}

func (c *Client) GetMessages(ctx context.Context, details *sms.PhoneNumber) ([]string, error) {
	metadata, ok := details.Metadata.(metadata)
	if !ok {
		return nil, sms.ErrInvalidMetadata
	}
	var data getMessagesResponse

	if err := c.do(ctx, url.Values{
		"metod":   {"get_sms"},
		"country": {metadata.country},
		"service": {metadata.service},
		"id":      {metadata.id},
	}, &data); err != nil {
		return nil, err
	}

	if data.Response == "2" {
		// sms: null (no messages yet)
		return []string{}, nil
	}

	if data.Response != "1" {
		return nil, fmt.Errorf("smspva: get_sms bad response %+v", data)
	}

	return []string{data.Text}, nil
}
