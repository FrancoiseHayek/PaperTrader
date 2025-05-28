package algorithms

import (
	"log"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/shopspring/decimal"
)

func SimpleAlgo(
	barCh <-chan stream.Bar,
	orderCh chan<- alpaca.PlaceOrderRequest,
) {

	for bar := range barCh {
		log.Printf("Received bar: %s - %.2f (O) → %.2f (C)\n", bar.Symbol, bar.Open, bar.Close)

		qty := decimal.NewFromInt(1)

		buyOrder := alpaca.PlaceOrderRequest{
			Symbol:      bar.Symbol,
			Qty:         &qty,
			Side:        alpaca.Buy,
			Type:        alpaca.Market,
			TimeInForce: alpaca.Day,
		}

		orderCh <- buyOrder

		time.Sleep(time.Second * 30)

		sellOrder := alpaca.PlaceOrderRequest{
			Symbol:      bar.Symbol,
			Qty:         &qty,
			Side:        alpaca.Sell,
			Type:        alpaca.Market,
			TimeInForce: alpaca.Day,
		}

		orderCh <- sellOrder

	}
}
