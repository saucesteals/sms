package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/saucesteals/sms/internal/gen"
	"github.com/saucesteals/sms/smspool"
)

func main() {
	client := smspool.NewClient(os.Args[1])

	smspoolServices, err := client.GetServices(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	var services []gen.Service
	for _, target := range smspoolServices {
		services = append(services, gen.Service{
			Name:  gen.Normalize(target.Name),
			Value: strconv.Itoa(target.Id),
		})
	}

	if err := gen.Generate("./services.go", gen.Data{Services: services, Package: "smspool"}); err != nil {
		log.Fatal(err)
	}
}
