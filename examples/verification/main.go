package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"time"

	"github.com/nyaruka/phonenumbers"
	"github.com/saucesteals/sms"
	"github.com/saucesteals/sms/smsman"
	"github.com/saucesteals/sms/smspva"
	"github.com/saucesteals/sms/textverified"
	"github.com/saucesteals/sms/truverifi"
)

var (
	provider = flag.String("provider", "", "phone number provider")
	apiKey   = flag.String("apikey", "", "api key for provider")
	service  = flag.String("service", "", "service to verify for")
	country  = flag.String("country", "", "country code to use")

	matcher = sms.NewMatcher(messageMatcher, time.Second, time.Minute)
)

func messageMatcher(message string) string {
	// match entire message
	return message
}

func main() {
	flag.Parse()

	ctx := context.Background()

	var client sms.Client
	switch *provider {
	case "smsman":
		client = smsman.NewClient(*apiKey)
	case "truverifi":
		client = truverifi.NewClient(*apiKey)
	case "smspva":
		client = smspva.NewClient(*apiKey)
	case "textverified":
		client = textverified.NewClient(*apiKey)

		// to avoid race between KeepAuthAlive authenticating and GetPhoneNumber needing authentication
		if err := client.(*textverified.Client).Authenticate(ctx); err != nil {
			log.Fatalf("textverified failed to authenticate: %q", err)
		}

		go func() {
			if err := client.(*textverified.Client).KeepAuthAlive(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				log.Fatalf("textverified failed to authenticate: %q", err)
			}
		}()
	default:
		log.Fatalf("unsupported provider %q", *provider)
	}

	phone, err := client.GetPhoneNumber(ctx, *service, *country)
	if err != nil {
		log.Fatal(err)
	}
	defer client.CancelPhoneNumber(context.Background(), phone) // Background() - ensure it gets cancelled regardless of ctx

	log.Printf("got phone number: %s", phone.Format(phonenumbers.INTERNATIONAL))

	message, err := matcher.WaitForMessage(ctx, client, phone)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("got message: %s", message)
}
