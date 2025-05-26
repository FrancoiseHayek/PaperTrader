package main

import (
	"fmt"
	"os"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/joho/godotenv"
)

func main() {

	// Load key environemnt variables
	godotenv.Load("../.env")
	API_KEY := os.Getenv("ALPACA_API_KEY")
	API_SECRET := os.Getenv("ALPACA_API_SECRET")
	BASE_URL := os.Getenv("ALPACA_BASE_URL")

	client := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    API_KEY,
		APISecret: API_SECRET,
		BaseURL:   BASE_URL,
	})

	acct, err := client.GetAccount()

	if err != nil {
		panic(err)
	}

	fmt.Printf("Accound status: %s\n", acct.Status)
	fmt.Printf("Buying Power: %s\n", acct.BuyingPower)

}
