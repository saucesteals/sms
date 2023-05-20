package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/saucesteals/sms/internal/gen"
	"github.com/saucesteals/sms/textverified"
)

func main() {
	client := textverified.NewClient(os.Args[1])

	if err := client.Authenticate(context.Background()); err != nil {
		log.Fatal(err)
	}

	targets, err := client.GetTargets(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	var services []gen.Service
	for _, target := range targets {
		services = append(services, gen.Service{
			Name:  gen.Normalize(target.Name),
			Value: strconv.Itoa(target.TargetID),
		})
	}

	if err := gen.Generate("./services.go", gen.Data{Services: services, Package: "textverified"}); err != nil {
		log.Fatal(err)
	}
}
