package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/saucesteals/sms/internal/gen"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func main() {
	in, err := os.Open("./gen/services.json")
	if err != nil {
		log.Fatal(err)
	}

	var services []struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}
	if err := json.NewDecoder(in).Decode(&services); err != nil {
		log.Fatal(err)
	}

	seen := map[string]struct{}{}
	var normalized []gen.Service
	for _, s := range services {
		s.Name = cases.Title(language.AmericanEnglish).String(gen.Normalize(s.Name))
		if _, ok := seen[s.Name]; ok {
			continue
		}
		seen[s.Name] = struct{}{}

		normalized = append(normalized, gen.Service{
			Name:  s.Name,
			Value: s.Code,
		})
	}

	if err := gen.Generate("./services.go", gen.Data{Services: normalized, Package: "smspva"}); err != nil {
		log.Fatal(err)
	}
}
