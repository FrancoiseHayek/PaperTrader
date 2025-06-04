package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/FrancoiseHayek/PaperTrader/algorithms"
	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/joho/godotenv"
)

func main() {

	// Load key environemnt variables
	if err := godotenv.Load("../.env"); err != nil {
		log.Printf("Could not load environment variables: %v", err)
	}

	// Create channels for bars and orders
	barCh := make(chan stream.Bar)
	orderCh := make(chan alpaca.PlaceOrderRequest)

	mdCtx, mdCancel := context.WithCancel(context.Background())
	defer mdCancel()
	tdCtx, tdCancel := context.WithCancel(context.Background())
	defer tdCancel()

	// Create a trading client
	tradingClient := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:     os.Getenv("APCA_API_KEY_ID"),
		APISecret:  os.Getenv("APCA_API_SECRET_KEY"),
		BaseURL:    os.Getenv("APCA_BASE_URL"),
		RetryLimit: 3,
		RetryDelay: 200 * time.Millisecond,
	})

	if _, err := tradingClient.GetAccount(); err != nil {
		log.Fatalf("Failire to connect to account: %v", err)
	}

	tradingClient.StreamTradeUpdatesInBackground(tdCtx, tradeUpdateHandler)

	// Check if market is open
	clock, clockErr := tradingClient.GetClock()
	if clockErr != nil {
		fmt.Printf("Clock error: %v", clockErr)
	}

	if clock.IsOpen {
		fmt.Println("Market is OPEN")
	} else {
		fmt.Printf("Market is CLOSED, next open on: %v", clock.NextOpen)
	}

	// Create a streaming client for market data
	marketDataClient := stream.NewStocksClient(marketdata.IEX)

	// Connect to the WebSocket stream
	if err := marketDataClient.Connect(mdCtx); err != nil {
		log.Printf("Failed to connext: %v", err)
	}

	// Subscibe to real-time bar updates for SPY
	go func() {
		subscribeErr := marketDataClient.SubscribeToBars(func(bar stream.Bar) {
			barCh <- bar
		}, "SPY")

		if subscribeErr != nil {
			log.Printf("failed to subscribe to SPY bars: %v", subscribeErr)
		}
	}()

	// Run algorithm
	go algorithms.SimpleAlgo(barCh, orderCh)

	// Consume orders and send to Alpaca
	go func() {
		for orderReq := range orderCh {
			_, err := tradingClient.PlaceOrder(orderReq)
			if err != nil {
				log.Printf("Unable to place order: %v", err)
				continue
			}
		}
	}()

	// Keep program running until a keyboard interrupt
	select {
	case err := <-marketDataClient.Terminated():
		log.Printf("Market Data stream terminated: %v", err)

	case <-waitForInterrupt():
		log.Println("Interrupt received. Shutting down...")
		tdCancel()
		mdCancel()

		// Wait for clean termination
		<-marketDataClient.Terminated()
		log.Println("Shutdown complete.")
	}
}

func waitForInterrupt() <-chan os.Signal {

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	return sig
}

func tradeUpdateHandler(update alpaca.TradeUpdate) {

	fmt.Printf("Trade Update Received: %v\n", update.Event)

	switch update.Event {
	case "fill":
		fmt.Printf("Order %s filled. Qty: %s at Price: %s\n",
			update.Order.ID,
			update.Order.FilledQty,
			update.Order.FilledAvgPrice,
		)
	case "pending_new":
		fmt.Printf("New order pending: %s\n", update.Order.ID)
	case "new":
		fmt.Printf("New order submitted: %s\n", update.Order.ID)
	case "canceled":
		fmt.Printf("Order %s canceled\n", update.Order.ID)
	case "rejected":
		fmt.Printf("Order %s was rejected: %s\n", update.Order.ID, update.Order.FailedAt)
	default:
		fmt.Printf("Unhandled update event: %+v\n", update)
	}
}
