package search

import (
	"log"

	"github.com/elastic/go-elasticsearch/v8"
)

var ES *elasticsearch.Client

func InitElastic() {
	cfg := elasticsearch.Config{
		Addresses: []string{
			"http://localhost:8080",
		},
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ES = client

}
