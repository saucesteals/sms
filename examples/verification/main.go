package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/nyaruka/phonenumbers"
	"github.com/saucesteals/sms"
	"github.com/saucesteals/sms/smspva"
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

	var client sms.Client

	switch *provider {
	case "truverifi":
		client = truverifi.NewClient(*apiKey)
	case "smspva":
		client = smspva.NewClient(*apiKey)
	default:
		log.Fatalf("unsupported provider %q", *provider)
	}

	ctx := context.Background()

	phone, err := client.GetPhoneNumber(ctx, *service, *country)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("got phone number: %s", phone.Format(phonenumbers.INTERNATIONAL))

	message, err := matcher.WaitForMessage(ctx, client, phone)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("got message: %s", message)
}
