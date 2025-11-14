package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"syscall/js"
)

// --- ВЕСЬ ТВОЙ СТАРЫЙ КОД (ПЕРЕМЕННЫЕ И ПАРСЕРЫ) ---
// (Я скопировал их из worker-parser/main.go, который мы чинили)

type MarketItem struct {
	Price  float64 `json:"price"`
	Volume int     `json:"volume"`
}

var priceMap = make(map[string]MarketItem)
var priceMapMutex = &sync.RWMutex{}

// (Тут будет структура CombinedItem из buff_parser.go)
// (Тут будут все функции из buff_parser.go и steam_liquid_tracker.go)
// ...
// (И эта функция:)
func fetchMarketPrices() {
	log.Println("🔄 Обновляю цены Market.csgo.com...")
	resp, err := http.Get("https://market.csgo.com/api/v2/prices/orders/RUB.json")
	if err != nil {
		log.Printf("❌ Ошибка: Не удалось загрузить цены Market: %v", err)
		return
	}
	defer resp.Body.Close()
	// ... (вся остальная логика fetchMarketPrices) ...
	var marketResponse struct {
		Success bool `json:"success"`
		Items   []struct {
			MarketHashName string  `json:"market_hash_name"`
			Price          float64 `json:"price"`
			Volume         int     `json:"volume"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&marketResponse); err != nil {
		log.Printf("❌ Ошибка: Не удалось расшифровать JSON от Market: %v", err)
		return
	}
	if !marketResponse.Success || len(marketResponse.Items) == 0 {
		log.Println("❌ Ошибка: Ответ от Market пришел, но он 'false' или пустой.")
		return
	}
	newPriceMap := make(map[string]MarketItem)
	for _, item := range marketResponse.Items {
		newPriceMap[item.MarketHashName] = MarketItem{
			Price:  item.Price,
			Volume: item.Volume,
		}
	}
	priceMapMutex.Lock()
	priceMap = newPriceMap
	priceMapMutex.Unlock()
	log.Printf("✅ Успешно! Цены Market (RUB) сохранены. Загружено %d предметов.", len(priceMap))
}

// --- НОВЫЙ "МОСТ" ДЛЯ ДВУХ ФУНКЦИЙ ---

func main() {
	c := make(chan struct{}, 0)
	// "Экспортируем" ОБЕ функции в JavaScript
	js.Global().Set("runAnalysis", js.FuncOf(runAnalysisGo))
	js.Global().Set("filterItems", js.FuncOf(filterItemsGo))
	<-c
}

// ❗️ ФУНКЦИЯ 1: ПАРСЕР (запускается по Cron)
// Он больше не пишет в R2, он ВОЗВРАЩАЕТ JSON
func runAnalysisGo(this js.Value, args []js.Value) interface{} {
	log.Println("🚀 (Go WASM) Запускаю парсинг...")

	loadProxies()
	createHTTPClient()
	fetchMarketPrices()
	RefreshTopLiquidAndComputeProfit(111)

	log.Println("✅ (Go WASM) Анализ завершен. Предметов найдено:", len(latestAnalysisResults))

	analysisMutex.RLock()
	data, err := json.Marshal(latestAnalysisResults)
	analysisMutex.RUnlock()

	if err != nil {
		log.Println("❌ (Go WASM) Ошибка кодирования JSON:", err)
		return "" // Возвращаем пустую строку в JS
	}

	log.Println("✅ (Go WASM) Результат парсинга (JSON) возвращен в JS.")
	return string(data) // Возвращаем JSON как строку
}

// ❗️ ФУНКЦИЯ 2: ФИЛЬТР (запускается по API)
// (Этот код мы уже писали для worker-api, он без изменений)
func filterItemsGo(this js.Value, args []js.Value) interface{} {
	itemsJSON := args[0].String()
	params := args[1]

	minProfit, _ := strconv.ParseFloat(params.Get("min_profit").String(), 64)
	minPrice, _ := strconv.ParseFloat(params.Get("min_price").String(), 64)
	maxPrice, err := strconv.ParseFloat(params.Get("max_price").String(), 64)
	if err != nil {
		maxPrice = 9999999.0
	}

	var allItems []CombinedItem
	if err := json.Unmarshal([]byte(itemsJSON), &allItems); err != nil {
		log.Println("❌ (Go WASM API) Ошибка парсинга JSON:", err)
		return ""
	}

	var filteredItems []CombinedItem
	for _, item := range allItems {
		if item.ProfitPercent >= minProfit &&
			item.MarketPrice >= minPrice &&
			item.MarketPrice <= maxPrice {
			filteredItems = append(filteredItems, item)
		}
	}

	filteredJSON, err := json.Marshal(filteredItems)
	if err != nil {
		log.Println("❌ (Go WASM API) Ошибка упаковки ответа:", err)
		return ""
	}

	log.Printf("✅ (Go WASM API) Найдено %d, отфильтровано %d", len(allItems), len(filteredItems))
	return string(filteredJSON)
}
