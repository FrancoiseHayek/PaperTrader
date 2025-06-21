package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/FrancoiseHayek/PaperTrader/algorithms"
	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
)

func main() {

	runtime.GOMAXPROCS(runtime.NumCPU())

	// CLI Flags
	symbol := flag.String("symbol", "", "Ticker symbol (e.g. SPY)")
	start := flag.String("start", "", "Start date YYYY-MM-DD")
	end := flag.String("end", "", "End date YYYY-MM-DD")
	flag.Parse()
	if *symbol == "" || *start == "" || *end == "" {
		log.Fatal("Must specify --symbol, --start, --end")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load or fetch CSV
	fileName := fmt.Sprintf("%s_%s_%s.csv", *symbol, *start, *end)
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		if err := fetchAndSaveBars(*symbol, *start, *end, fileName); err != nil {
			log.Fatalf("fetch bars: %v", err)
		}
	}

	// Channels
	algoBarCh := make(chan stream.Bar)
	execBarCh := make(chan stream.Bar)
	orderCh := make(chan alpaca.PlaceOrderRequest, 100)
	updateCh := make(chan alpaca.TradeUpdate)
	logCh := make(chan string)

	state := &algorithms.State{}

	// Run engine
	go func() {

		state = algorithms.Execute_RSI_Bullish(ctx, logCh, algoBarCh, orderCh, updateCh)

	}()

	numPositionsOpen := []int{0}
	numTrades := 0

	// Drain logs
	go func() {
		for range logCh {
			<-logCh
		}
	}()

	// Feeder: read CSV rows → broadcast into both channels
	go func() {
		f, err := os.Open(fileName)
		if err != nil {
			logCh <- fmt.Sprintf("open CSV: %v", err)
			return
		}
		defer f.Close()
		r := csv.NewReader(f)
		// skip header
		if _, err := r.Read(); err != nil {
			return
		}
		for {
			record, err := r.Read()
			if err == io.EOF {
				cancel()
				close(execBarCh)
				close(algoBarCh)
				break
			} else if err != nil {
				logCh <- fmt.Sprintf("csv read: %v", err)
				break
			}
			t, _ := time.Parse(time.RFC3339, record[0])
			open, _ := strconv.ParseFloat(record[1], 64)
			high, _ := strconv.ParseFloat(record[2], 64)
			low, _ := strconv.ParseFloat(record[3], 64)
			closep, _ := strconv.ParseFloat(record[4], 64)
			vol, _ := strconv.ParseUint(record[5], 10, 64)

			bar1 := stream.Bar{
				Timestamp: t, Open: open, High: high, Low: low, Close: closep, Volume: vol,
			}

			bar2 := stream.Bar{
				Timestamp: t, Open: open, High: high, Low: low, Close: closep, Volume: vol,
			}

			algoBarCh <- bar1
			execBarCh <- bar2
		}
	}()

	// Handle orders and bar-driven sells
	go func() {
		var sellOrders []alpaca.PlaceOrderRequest
		var lastBar stream.Bar
		for {
			select {
			case <-ctx.Done():
			case bar := <-execBarCh:

				lastBar = bar
				i := 0
				for i < len(sellOrders) {

					order := sellOrders[i]
					target := order.LimitPrice
					if decimalOr(lastBar.High).GreaterThanOrEqual(*target) {

						ntnl := order.Qty.Mul(*order.LimitPrice)
						trupd := alpaca.TradeUpdate{
							Event: "fill",
							Order: alpaca.Order{
								Symbol:         order.Symbol,
								Side:           order.Side,
								FilledAvgPrice: target,
								FilledQty:      *order.Qty,
								Notional:       &ntnl,
							},
						}

						// updateCh <- trupd // Removed for suspicion of circular dependency causing deadlock

						state.Update(func(s *algorithms.State) {
							s.Cash += trupd.Order.Notional.InexactFloat64()
							s.OpenPositions--
							s.SharesHeld -= trupd.Order.FilledQty.InexactFloat64()
						})

						sellOrders = append(sellOrders[:i], sellOrders[i+1:]...)

						n := len(numPositionsOpen)
						numPositionsOpen = append(numPositionsOpen, numPositionsOpen[n-1]-1)
						numTrades += 1
					} else {
						i++
					}
				}
			case ord, ok := <-orderCh:
				if !ok {
					return
				}

				if ord.Side == alpaca.Buy {

					// simulate immediate buy at last close
					price := decimal.NewFromFloat(lastBar.Close)
					trupd := alpaca.TradeUpdate{
						Event: "fill",
						Order: alpaca.Order{
							Symbol:         ord.Symbol,
							Side:           ord.Side,
							FilledAvgPrice: &price,
							FilledQty:      ord.Notional.Div(price),
							Notional:       ord.Notional,
						},
					}
					updateCh <- trupd

					n := len(numPositionsOpen)
					numPositionsOpen = append(numPositionsOpen, numPositionsOpen[n-1]+1)

				} else if ord.Side == alpaca.Sell { // if sell, monitor future bars
					sellOrders = append(sellOrders, ord)
				}
			}
		}
	}()

	// wait until bars are done
	<-ctx.Done()

	time.Sleep(time.Second * 3)

	close(logCh)
	close(orderCh)
	close(updateCh)

	// write results
	out := struct {
		State               *algorithms.State
		AverageNumPositions int
		NumTrades           int
	}{
		State:               state,
		AverageNumPositions: avg(numPositionsOpen),
		NumTrades:           numTrades,
	}
	resFile := fmt.Sprintf("results_%s_%s_%s.json", *symbol, *start, *end)
	f, _ := os.Create(resFile)
	defer f.Close()
	json.NewEncoder(f).Encode(out)
}

// fetchAndSaveBars: fetch via Alpaca REST and save CSV
func fetchAndSaveBars(sym, start, end, file string) error {

	// Load key environemnt variables
	if err := godotenv.Load("../.env"); err != nil {
		fmt.Printf("Could not load environment variables: %v", err)
	}

	client := marketdata.NewClient(marketdata.ClientOpts{
		APIKey:    os.Getenv("APCA_API_KEY_ID"),
		APISecret: os.Getenv("APCA_API_SECRET_KEY"),
	})

	startDate, _ := time.Parse("2006-01-02", start)
	endDate, _ := time.Parse("2006-01-02", end)

	bars, err := client.GetBars(sym, marketdata.GetBarsRequest{
		TimeFrame: marketdata.NewTimeFrame(1, "Min"),
		Start:     startDate,
		End:       endDate,
	})

	if err != nil {
		return err
	}

	f, _ := os.Create(file)
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"timestamp", "open", "high", "low", "close", "volume"})
	for _, b := range bars {
		w.Write([]string{
			b.Timestamp.Format(time.RFC3339),
			fmt.Sprint(b.Open), fmt.Sprint(b.High),
			fmt.Sprint(b.Low), fmt.Sprint(b.Close),
			fmt.Sprint(b.Volume),
		})
	}
	return nil
}

// decimalOr helper
func decimalOr(f float64) decimal.Decimal {
	return decimal.NewFromFloat(f)
}

func avg(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	var sum int
	for _, v := range xs {
		sum += v
	}
	return int(math.Floor(float64(sum) / float64(len(xs))))
}
