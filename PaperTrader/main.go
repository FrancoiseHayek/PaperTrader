package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"

	"github.com/FrancoiseHayek/PaperTrader/algorithms"
	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/joho/godotenv"
)

func main() {

	log.Println("Starting up.")

	runtime.GOMAXPROCS(runtime.NumCPU())

	// Keep program running until a keyboard interrupt
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	// Load key environemnt variables
	if err := godotenv.Load("../.env"); err != nil {
		log.Printf("Could not load environment variables: %v", err)
	}

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

	// Check if market is open
	clock, clockErr := tradingClient.GetClock()
	if clockErr != nil {
		fmt.Printf("Clock error: %v", clockErr)
	}

	if clock.IsOpen {
		log.Println("Market is OPEN")
	} else {
		log.Printf("Market is CLOSED, next open on: %v\nExiting...\n", clock.NextOpen)
	}

	// Create channels for bars and orders
	barCh := make(chan stream.Bar)
	orderCh := make(chan alpaca.PlaceOrderRequest)
	logCh := make(chan string, 150)
	updateCh := make(chan alpaca.TradeUpdate)

	logCtx, logCancel := context.WithCancel(context.Background())
	defer logCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(5)

	// Logger
	go func(logCtx context.Context, logCh chan string) {
		defer wg.Done()
		log.Println("Starting Logger...")

		for {
			select {
			case <-logCtx.Done():
				log.Println("Logger shutting down...")
				return

			case msg := <-logCh:
				log.Println(msg)
			}
		}
	}(logCtx, logCh)

	// Market data
	go func(ctx context.Context, logCh chan string) {
		defer wg.Done()

		logCh <- "Calling algorithm's market data function..."

		logCh <- "Market data stream shut down..."

	}(ctx, logCh)

	// Algorithm
	go func(ctx context.Context, logCh chan string, barCh chan stream.Bar, orderCh chan alpaca.PlaceOrderRequest, updateCh chan alpaca.TradeUpdate) {
		defer wg.Done()
		logCh <- "Starting Algorithm..."
		algorithms.Execute_RSI_Bullish(ctx, logCh, barCh, orderCh, updateCh)
	}(ctx, logCh, barCh, orderCh, updateCh)

	// Trade data
	go func(ctx context.Context, tradingClient *alpaca.Client, logCh chan string) {
		defer wg.Done()

		logCh <- "Starting Trade update stream..."

		tradingClient.StreamTradeUpdatesInBackground(ctx, func(update alpaca.TradeUpdate) {

			msg := fmt.Sprintf("Trade Update Received: %v. ", update.Event)

			switch update.Event {
			case "fill":
				updateCh <- update
				msg += fmt.Sprintf("Order %s filled. Qty: %s at Price: %s",
					update.Order.ID,
					update.Order.FilledQty,
					update.Order.FilledAvgPrice)
			case "pending_new":
				msg += fmt.Sprintf("New order pending: %s", update.Order.ID)
			case "new":
				msg += fmt.Sprintf("New order submitted: %s", update.Order.ID)
			case "canceled":
				msg += fmt.Sprintf("Order %s canceled", update.Order.ID)
			case "rejected":
				msg += fmt.Sprintf("Order %s was rejected: %s", update.Order.ID, update.Order.FailedAt)
			default:
				msg += fmt.Sprintf("Unhandled update event: %+v", update)
			}

			logCh <- msg
		})

		// Wait until the context is canceled (program is terminated)
		<-ctx.Done()

		logCh <- "Trade data stream shutting down..."
	}(ctx, tradingClient, logCh)

	// Trade client
	go func(ctx context.Context, orderCh chan alpaca.PlaceOrderRequest, tradingClient *alpaca.Client, logCh chan string) {
		defer wg.Done()

		logCh <- "Starting Trading stream..."

		// Consume orders and send to Alpaca API

		for {
			select {
			case <-ctx.Done():
				logCh <- "Trading stream shutting down..."
				return
			case orderReq := <-orderCh:
				_, err := tradingClient.PlaceOrder(orderReq)
				if err != nil {
					msg := fmt.Sprintf("Unable to place order: %v", err)
					logCh <- msg
				}
			}
		}
	}(ctx, orderCh, tradingClient, logCh)

	<-sig

	logCh <- "Keyboard interrupt received, Shutting down..."

	cancel()
	time.Sleep(time.Second * 3)
	logCancel()
	wg.Wait()

	log.Println("Shutdown complete.")
}
