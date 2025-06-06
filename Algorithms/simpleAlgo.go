package algorithms

import (
	"context"
	"fmt"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/shopspring/decimal"
)

func SimpleAlgo(
	ctx context.Context,
	barCh <-chan stream.Bar,
	orderCh chan<- alpaca.PlaceOrderRequest,
	logCh chan<- string,
) {

	for {
		select {
		case <-ctx.Done():
			logCh <- "Algorithm shutting down..."
			return
		case bar := <-barCh:
			msg := fmt.Sprintf("Received bar: %s - %.2f (O) → %.2f (C)", bar.Symbol, bar.Open, bar.Close)
			logCh <- msg

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

}
