package main

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"bot/go_kraken/rest"
)

// TradingViewWebhookPayload defines the expected structure of the JSON payload
type TradingViewWebhookPayload struct {
	// Add fields based on the expected JSON structure
	// Example:
	Event   string  `json:"event"`
	Price   float64 `json:"price"`
	Message string  `json:"message"`
	Action  string  `json:"action"`
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	// Ensure the request method is POST
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		slog.Debug("invalid method")
		return
	}

	// Parse the JSON payload
	var payload TradingViewWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		slog.Debug("invalid json", "error", err)
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	go B.trade(payload)
	// Process the payload
	fmt.Printf("Received webhook: Event=%s, Price=%.2f, Message=%s, Action=%s\n", payload.Event, payload.Price, payload.Message, payload.Action)

	// Respond to TradingView
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Webhook received successfully"))
}

var B Trader

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	api := rest.New("EIXlKfdefCBbrB1aO3LN/ma51012MCy8WjoXgaAWfxwF934RphyvGE22", "sfAh5ycZIW2Xl8Zd8peAr1UlZjPrqMhG5SyfJFzgu60xNlN9ZI9mEF+nFTUfv3MfCkvqlhtpOrZ+A352dohoVQ==")
	B = Trader{cli: api}
	http.HandleFunc("/webhook", webhookHandler)

	// Start the server
	port := "8080" // Change to your desired port
	fmt.Printf("Server starting on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

type Trader struct {
	cli  *rest.Kraken
	side string
	mu   sync.Mutex
}

func (t *Trader) trade(payload TradingViewWebhookPayload) {
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
		if payload.Action == "buy" {
			t.side = "buy"
			candles, _ := t.cli.Candles("SOLUSD", 1, time.Now().Add(time.Second).Unix())
			tickerRES, _:= t.cli.Ticker("SOLUSD")
			limitPrice,_ := tickerRES["SOLUSD"].Bid.Price.Float64()
			_, _ = candles.Candles["SOLUSD"][0].Close.Float64()
			zusd, _ := bals["ZUSD"].Float64()
			quantity := (zusd * 0.95) / limitPrice
			_, err := t.cli.AddOrder("SOLUSD",
				"buy",
				"limit",
				quantity,
				map[string]any{
					"price":            limitPrice + 0.05,
					//"price2":           limitPrice + 2.5,
					//"close[ordertype]": "stop-price",
					// "close[price]":     limitPrice - 0.05,
				})
			if err != nil {
				slog.Error("error placing buy trade", "error", err)
			}
			t.mu.Unlock()
		}

	} else if strings.ToLower(payload.Action) == t.side {
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
		sols, _ := bals["SOL"].Float64()
		_, err = t.cli.AddOrder("SOLUSD",
				"sell",
				"market",
				sols,
				map[string]any{
					//"price":            limitPrice + 0.05,
					//"price2":           limitPrice + 2.5,
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

func placeOrder(price float64){}
