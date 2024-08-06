package main

import (
	"bot/go_kraken/rest"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/markcheno/go-talib"
)

var B Trader

// Define the structure to unmarshal the Kraken API response
type KrakenResponse struct {
	Result map[string]any `json:"result"`
	Last   int            `json:"last"`
}

// Define the structure to unmarshal the Coinbase API response
type CoinbaseResponse struct {
	Last string `json:"last"`
}

const (
	krakenSymbol        = "SOLUSD"
	coinbaseSymbol      = "SOLUSD"
	zPeriod             = 35
	atrPeriod           = 35
	threshold           = 1.95
	zScoreExitThreshold = 1.95
	accountBalance      = 2500
)

// Function to fetch data from Kraken API
func fetchKrakenData(url string, wg *sync.WaitGroup, prices chan<- float64) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		slog.Error("Failed to fetch data from ", "url", url, "error", err)
		prices <- 0.0
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Failed to read response body from ", "url", url, "error", err)
		prices <- 0.0
		return
	}

	var kr KrakenResponse
	err = json.Unmarshal(body, &kr)
	if err != nil {
		slog.Error("Failed to unmarshal JSON from", "url", url, "error", err)
		prices <- 0.0
		return
	}

	// Extract the latest price from the Kraken response
	latestData := kr.Result["SOLUSD"].([]any)
	if len(latestData) > 0 {
		latestPriceStr := latestData[len(latestData)-1].([]any)[1].(string)
		latestPrice, err := strconv.ParseFloat(latestPriceStr, 64)
		if err != nil {
			slog.Error("Failed to parse price from", "url", url, "error", err)
			prices <- 0.0
			return
		}
		prices <- latestPrice
		return
	}

	prices <- 0.0
}

// Function to fetch data from Coinbase API
func fetchCoinbaseData(url string, wg *sync.WaitGroup, prices chan<- float64) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		slog.Error("Failed to fetch data from", "url", url, "error", err)
		prices <- 0.0
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Failed to read response body from", "url", url, "error", err)
		prices <- 0.0
		return
	}

	var cr CoinbaseResponse
	err = json.Unmarshal(body, &cr)
	if err != nil {
		slog.Error("Failed to unmarshal JSON from", "url", url, "error", err)
		prices <- 0.0
		return
	}

	priceFloat, err := strconv.ParseFloat(cr.Last, 64)
	if err != nil {
		slog.Error("Failed to parse price from", "url", url, "error", err)
		prices <- 0.0
		return
	}

	prices <- priceFloat
}

var data = NewCircularBuffer(zPeriod + 1)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	api := rest.New("EIXlKfdefCBbrB1aO3LN/ma51012MCy8WjoXgaAWfxwF934RphyvGE22", "sfAh5ycZIW2Xl8Zd8peAr1UlZjPrqMhG5SyfJFzgu60xNlN9ZI9mEF+nFTUfv3MfCkvqlhtpOrZ+A352dohoVQ==")
	B = Trader{cli: api}
	// Define URLs
	krakenURL := "https://api.kraken.com/0/public/Spread?pair=SOLUSD"
	coinbaseURL := "https://api.exchange.coinbase.com/products/SOL-USD/stats"

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		startTime := time.Now()

		var wg sync.WaitGroup
		prices := make(chan float64, 2)

		wg.Add(2)
		go fetchKrakenData(krakenURL, &wg, prices)
		go fetchCoinbaseData(coinbaseURL, &wg, prices)

		wg.Wait()
		close(prices)

		// Read prices from the channel
		var krakenPrice, coinbasePrice float64
		count := 0
		for price := range prices {
			if count == 0 {
				krakenPrice = price
			} else {
				coinbasePrice = price
			}
			count++
		}

		// Calculate and print the price difference
		priceDifference := krakenPrice - coinbasePrice
		slog.Info("Kraken Price: ", "price", krakenPrice)
		slog.Info("Coinbase Price: ", "price", coinbasePrice)
		slog.Info("Price Difference: ", "price", priceDifference)
		data.Add(priceDifference)
		if data.Full() {
			_, _, zScore, atrZScore := calculateIndicators(data.Values())

			// Define strategy conditions
			longCondition := zScore < -threshold && atrZScore < -threshold
			longExitCondition := zScore > zScoreExitThreshold

			// Implement strategy logic
			if longCondition {
				// Place buy order
				go B.trade("buy")
			}

			if longExitCondition {
				// Place sell order
				go B.trade("sell")
			}

			// Plotting and alerting logic can be implemented similarly using libraries or APIs that support these features
			slog.Info("Strategy executed")
		}
		// Calculate and print the time taken
		elapsedTime := time.Since(startTime)
		slog.Info("Time taken:", "time", elapsedTime)
	}
}

func calculateIndicators(data []float64) (float64, float64, float64, float64) {
	sma := talib.Sma(data, zPeriod)
	stdev := talib.StdDev(data, zPeriod, 1)
	zScore := (data[len(data)-1] - sma[len(sma)-1]) / stdev[len(stdev)-1]
	atr := talib.Atr(data, data, data, atrPeriod)
	atrZScore := (data[len(data)-1] - sma[len(sma)-1]) / atr[len(atr)-1]
	return sma[len(sma)-1], stdev[len(stdev)-1], zScore, atrZScore
}

// CircularBuffer is a fixed-size circular buffer
type CircularBuffer struct {
	data     []float64
	size     int
	position int
	full     bool
}

// NewCircularBuffer creates a new CircularBuffer
func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		data: make([]float64, size),
		size: size,
	}
}

// Add adds a new value to the buffer
func (cb *CircularBuffer) Add(value float64) {
	cb.data[cb.position] = value
	cb.position = (cb.position + 1) % cb.size
	if cb.position == 0 {
		cb.full = true
	}
}

// Values returns the values in the buffer in order
func (cb *CircularBuffer) Values() []float64 {
	if !cb.full {
		return cb.data[:cb.position]
	}
	return append(cb.data[cb.position:], cb.data[:cb.position]...)
}

func (cb *CircularBuffer) Full() bool {
	return cb.full
}

type Trader struct {
	cli  *rest.Kraken
	side string
	mu   sync.Mutex
}

func (t *Trader) trade(action string) {
	t.mu.Lock()
	if t.side == "" {
		slog.Info("no trade. Trading...")
		// No running trades
		bals, err := t.cli.GetAccountBalances()
		if err != nil {
			slog.Error("Error getting account balances", "error", err)
			t.mu.Unlock()
			return
		}
		if action == "buy" {
			t.side = "buy"
			candles, _ := t.cli.Candles("SOLUSD", 1, time.Now().Add(time.Second).Unix())
			tickerRES, _ := t.cli.Ticker("SOLUSD")
			limitPrice, _ := tickerRES["SOLUSD"].Bid.Price.Float64()
			_, _ = candles.Candles["SOLUSD"][0].Close.Float64()
			zusd, _ := bals["ZUSD"].Float64()
			quantity := (zusd * 0.95) / limitPrice
			_, err := t.cli.AddOrder("SOLUSD",
				"buy",
				"limit",
				quantity,
				map[string]any{
					"price": limitPrice + 0.01,
					//"price2":           limitPrice + 2.5,
					//"close[ordertype]": "stop-price",
					// "close[price]":     limitPrice - 0.05,
				})
			if err != nil {
				slog.Error("error placing buy trade", "error", err)
			}
			t.mu.Unlock()
		} else {
			t.mu.Unlock()
		}

	} else if strings.ToLower(action) == t.side {
		// Same side.Ignore
		t.mu.Unlock()
		return
	} else {
		// exit trade
		bals, err := t.cli.GetAccountBalances()
		if err != nil {
			slog.Error("Error getting account balances", "error", err)
			t.mu.Unlock()
			return
		}
		tickerRES, _ := t.cli.Ticker("SOLUSD")
		limitPrice, _ := tickerRES["SOLUSD"].Bid.Price.Float64()
		sols, _ := bals["SOL"].Float64()
		_, err = t.cli.AddOrder("SOLUSD",
			"sell",
			"stop-loss-limit",
			sols,
			map[string]any{
				"price":  limitPrice - 0.01,
				"price2": limitPrice - 0.01,
				//"close[ordertype]": "stop-price",
				// "close[price]":     limitPrice - 0.05,
			})
		if err != nil {
			slog.Error("error placing buy trade", "error", err)
		}
		//t.cli.CancelAll()
		t.side = ""
		t.mu.Unlock()

	}

}
