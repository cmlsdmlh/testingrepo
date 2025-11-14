package main

import (
	"encoding/json"
	"log"
	"net/http" // ❗️ ДОБАВЛЕН
	"sync"     // ❗️ ДОБАВЛЕН
	"syscall/js"
)

// --- ❗️ ДОБАВЛЕННЫЙ КОД ИЗ ТВОЕГО СТАРОГО main.go ---

type MarketItem struct {
	Price  float64 `json:"price"`
	Volume int     `json:"volume"`
}

var priceMap = make(map[string]MarketItem)
var priceMapMutex = &sync.RWMutex{}

func fetchMarketPrices() {
	log.Println("🔄 Обновляю цены Market.csgo.com...")

	// ❗️ Примечание: http.Get в WASM работает,
	// но он будет использовать 'fetch' из JavaScript,
	// который мы настроим в Cloudflare.
	resp, err := http.Get("https://market.csgo.com/api/v2/prices/orders/RUB.json")
	if err != nil {
		log.Printf("❌ Ошибка: Не удалось загрузить цены Market: %v", err)
		return
	}
	defer resp.Body.Close()

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

// --- ❗️ КОНЕЦ ДОБАВЛЕННОГО КОДА ---

// --- Это мой код из прошлого шага (без изменений) ---

// "Мост" в JavaScript для работы с Cloudflare
func main() {
	c := make(chan struct{}, 0)
	js.Global().Set("runAnalysis", js.FuncOf(runAnalysisGo))
	<-c
}

// Наша главная функция парсинга, обернутая для JS
func runAnalysisGo(this js.Value, args []js.Value) interface{} {
	log.Println("🚀 (Go WASM) Запускаю парсинг...")

	// --- 1. Выполняем твой код (теперь он найдет эти функции) ---
	loadProxies()      // Из buff_parser.go
	createHTTPClient() // Из buff_parser.go

	fetchMarketPrices()                   // ❗️ Вот эта функция, которую мы добавили
	RefreshTopLiquidAndComputeProfit(111) // Из steam_liquid_tracker.go

	log.Println("✅ (Go WASM) Анализ завершен. Предметов найдено:", len(latestAnalysisResults))

	// --- 2. Упаковываем результат в JSON ---
	analysisMutex.RLock()
	data, err := json.Marshal(latestAnalysisResults)
	analysisMutex.RUnlock()

	if err != nil {
		log.Println("❌ (Go WASM) Ошибка кодирования JSON:", err)
		return nil
	}

	log.Println("✅ (Go WASM) Результат упакован в JSON.")

	// --- 3. Сохраняем JSON в R2 (через "мост" JS) ---
	r2Bucket := js.Global().Get("RESULTS_BUCKET")
	if r2Bucket.IsUndefined() {
		log.Println("❌ (Go WASM) Не найден R2 бакет 'RESULTS_BUCKET'!")
		return nil
	}

	jsBuffer := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsBuffer, data)

	r2Bucket.Call("put", "latest-data.json", jsBuffer)

	log.Println("✅ (Go WASM) JSON отправлен в R2. Работа завершена.")
	return nil
}
