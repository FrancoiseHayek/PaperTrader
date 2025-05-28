package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
)

func main() {

	// Load key environemnt variables
	godotenv.Load("../.env")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a trading client
	tradingClient := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:     os.Getenv("APCA_API_KEY_ID"),
		APISecret:  os.Getenv("APCA_API_SECRET_KEY"),
		BaseURL:    os.Getenv("APCA_BASE_URL"),
		RetryLimit: 3,
		RetryDelay: 200 * time.Millisecond,
	})

	if _, err := tradingClient.GetAccount(); err != nil {
		log.Fatalf("Failire to connect account: %v", err)
	}

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

	// Create a streaming client
	marketDataClient := stream.NewStocksClient(marketdata.IEX)

	// Connect to the WebSocket stream
	if err := marketDataClient.Connect(ctx); err != nil {
		log.Fatalf("Failed to connext: %v", err)
	}

	// Subscibe to real-time bar updates for SPY
	go func() {
		subscribeErr := marketDataClient.SubscribeToBars(func(bar stream.Bar) {
			fmt.Printf("Bar - %s | Time: %v | Open: %.2f | High: %.2f | Low %.2f | Close: %.2f | Volume: %d | VWAP: %.2f\n",
				bar.Symbol, bar.Timestamp, bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.VWAP)
		}, "SPY")

		if subscribeErr != nil {
			log.Fatalf("failed to subscribe to SPY bars: %v", subscribeErr)
		}
	}()

	go func() {
		qty := decimal.NewFromInt(1)
		fmt.Println("Sleeping for 30s")
		time.Sleep(time.Second * 30)
		fmt.Println("Submitting order for SPY")

		// Submit a market buy order for 1 share of SPY
		buyOrder, buyErr := tradingClient.PlaceOrder(alpaca.PlaceOrderRequest{
			Symbol:      "SPY",
			Qty:         &qty,
			Side:        alpaca.Buy,
			Type:        alpaca.Market,
			TimeInForce: alpaca.Day,
		})
		if buyErr != nil {
			log.Fatalf("Failed to submit order: %v", buyErr)
		}

		fmt.Printf("Order submitted: ID=%s, Status=%s, Symbol=%s\n", buyOrder.ID, buyOrder.Status, buyOrder.Symbol)

		fmt.Println("Sleeping for another 30s")
		time.Sleep(time.Second * 30)
		fmt.Println("Submitting sell order")

		sellOrder, sellErr := tradingClient.PlaceOrder(alpaca.PlaceOrderRequest{
			Symbol:      "SPY",
			Qty:         &qty,
			Side:        alpaca.Sell,
			Type:        alpaca.Market,
			TimeInForce: alpaca.Day,
		})

		if sellErr != nil {
			log.Fatalf("Failed to submit sell order: %v", sellErr)
		}

		fmt.Printf("Order submitted: ID=%s, Status=%s, Symbol=%s\n", sellOrder.ID, sellOrder.Status, sellOrder.Symbol)

	}()

	// Keep program running until a keyboard interrupt
	select {
	case err := <-marketDataClient.Terminated():
		log.Printf("Stream terminated: %v", err)

	case <-waitForInterrupt():
		log.Println("Interrupt received. Shutting down...")
		cancel()

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
