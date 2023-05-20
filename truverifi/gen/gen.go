package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/saucesteals/sms/internal/gen"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func main() {
	in, err := os.Open("./gen/services.json")
	if err != nil {
		log.Fatal(err)
	}

	var serviceCodes []string
	if err := json.NewDecoder(in).Decode(&serviceCodes); err != nil {
		log.Fatal(err)
	}

	var services []gen.Service
	for _, s := range serviceCodes {
		services = append(services, gen.Service{
			Value: s,
			Name:  cases.Title(language.AmericanEnglish).String(strings.ToLower(gen.Normalize(s))),
		})
	}

	if err := gen.Generate("./services.go", gen.Data{Services: services, Package: "truverifi"}); err != nil {
		log.Fatal(err)
	}
}
