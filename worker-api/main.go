package main

import (
	"encoding/json"
	"log"
	"strconv"
	"syscall/js"
)

// ❗️ ВОТ СЮДА МЫ ДОБАВИЛИ СТРУКТУРУ (вместо целого файла) ❗️
type CombinedItem struct {
	Name               string  `json:"name"`
	BuffID             int     `json:"buff_id"`
	IconURL            string  `json:"icon_url"`
	Exterior           string  `json:"exterior"`
	BuffPrice          float64 `json:"buffPrice"`
	BuffSellNum        int     `json:"buffSellNum"`
	MarketPrice        float64 `json:"marketPrice"`
	MarketVolume       int     `json:"marketVolume"`
	SteamPrice         float64 `json:"steamPrice"`
	SteamMarketURL     string  `json:"steam_market_url"`
	ProfitPercent      float64 `json:"profitPercent"`
	ProfitRub          float64 `json:"profitRub"`
	Status             string  `json:"status"`
	ProfitSteamPercent float64 `json:"profitSteamPercent"`
	StatusSteam        string  `json:"statusSteam"`
	SteamPriceRaw      string  `json:"steam_price_raw"`
}

// --- Весь остальной код main.go (без изменений) ---

// "Мост" в JavaScript для работы с Cloudflare
func main() {
	c := make(chan struct{}, 0)
	js.Global().Set("filterItems", js.FuncOf(filterItemsGo))
	<-c
}

// Это единственная функция. Она принимает:
// 1. dataFromR2 (string) - Весь JSON из R2
// 2. params (js.Value) - JS-объект с параметрами
// ... и возвращает отфильтрованный JSON (string)
func filterItemsGo(this js.Value, args []js.Value) interface{} {
	log.Println("🚀 (Go WASM API) Запрос на фильтрацию...")

	// --- 1. Получаем данные ---
	itemsJSON := args[0].String()
	params := args[1]

	// Читаем параметры из JS-объекта
	minProfit, _ := strconv.ParseFloat(params.Get("min_profit").String(), 64)
	minPrice, _ := strconv.ParseFloat(params.Get("min_price").String(), 64)
	// (Если max_price пустой, ставим "бесконечность")
	maxPrice, err := strconv.ParseFloat(params.Get("max_price").String(), 64)
	if err != nil {
		maxPrice = 9999999.0
	}

	// --- 2. Распаковываем JSON ---
	var allItems []CombinedItem // ❗️ Теперь он найдет эту структуру
	if err := json.Unmarshal([]byte(itemsJSON), &allItems); err != nil {
		log.Println("❌ (Go WASM API) Ошибка парсинга JSON из R2:", err)
		return ""
	}

	// --- 3. ❗️ ТВОЯ ЛОГИКА ФИЛЬТРАЦИИ (из старого main.go) ---
	var filteredItems []CombinedItem // ❗️ И эту найдет
	for _, item := range allItems {
		if item.ProfitPercent >= minProfit &&
			item.MarketPrice >= minPrice &&
			item.MarketPrice <= maxPrice {
			filteredItems = append(filteredItems, item)
		}
	}
	// --- (Конец твоей логики) ---

	// --- 4. Упаковываем и возвращаем ---
	filteredJSON, err := json.Marshal(filteredItems)
	if err != nil {
		log.Println("❌ (Go WASM API) Ошибка упаковки ответа:", err)
		return ""
	}

	log.Printf("✅ (Go WASM API) Найдено %d, отфильтровано %d", len(allItems), len(filteredItems))
	return string(filteredJSON)
}
