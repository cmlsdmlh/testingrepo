package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ====== Настройки (подстрой под себя) ======
var (
	steamConcurrency = 44               // уменьшил concurrency по умолчанию (1-3 — безопаснее)
	steamCacheTTL    = 25 * time.Second // короткий TTL, чтобы ловить быстрые движения
	maxTopToQuery    = 300              // максимум для RefreshTop... по умолчанию
	httpClientSmall  = &http.Client{Timeout: 12 * time.Second}
	userAgentSteam   = "Mozilla/5.0 (Linux; Android 6.0; Nexus 5 Build/MRA58N) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Mobile Safari/537.36 Edg/142.0.0.0"
)

// Примечание: в main.go у тебя должна существовать функция createHTTPClient(), которая
// инициализирует глобальную переменную `httpClient`. Если её нет — добавь.
// Здесь мы только используем эту глобальную переменную (если она nil, используем httpClientSmall).

// ====== Кеш Steam ======
type steamCacheEntry struct {
	Price     float64
	Raw       string
	FetchedAt time.Time
	ExpiresAt time.Time
}

var (
	steamCache      = make(map[string]steamCacheEntry)
	steamCacheMutex = &sync.RWMutex{}
)

// ====== Вспомог: получить топ-N по объёму из priceMap ======
type marketEntry struct {
	Name   string
	Price  float64
	Volume int
}

func topNFromPriceMap(n int) []marketEntry {
	priceMapMutex.RLock()
	defer priceMapMutex.RUnlock()

	arr := make([]marketEntry, 0, len(priceMap))
	for name, it := range priceMap {
		arr = append(arr, marketEntry{Name: name, Price: it.Price, Volume: it.Volume})
	}
	sort.Slice(arr, func(i, j int) bool {
		return arr[i].Volume > arr[j].Volume
	})
	if n > len(arr) {
		n = len(arr)
	}
	return arr[:n]
}

// ====== Парсер строки цены Steam в float64 ======
var nonNumRe = regexp.MustCompile(`[^0-9\.,]`)

func parseSteamPriceString(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = nonNumRe.ReplaceAllString(s, "")
	if s == "" {
		return 0, fmt.Errorf("empty price string")
	}

	// Если есть и точка и запятая — удаляем запятые как разделители тысяч
	if strings.Contains(s, ".") && strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ",", "")
	} else if strings.Contains(s, ",") && !strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", ".")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse float fail '%s': %v", s, err)
	}
	return f, nil
}

// ====== Получить цену из кеша или из Steam (priceoverview) ======
func getSteamPriceCached(marketHashName string) (float64, string, error) {
	// 1) кеш
	steamCacheMutex.RLock()
	if e, ok := steamCache[marketHashName]; ok {
		if time.Now().Before(e.ExpiresAt) {
			steamCacheMutex.RUnlock()
			return e.Price, e.Raw, nil
		}
	}
	steamCacheMutex.RUnlock()

	// 2) fetch с retry
	price, raw, err := fetchSteamPriceWithRetry(marketHashName)
	if err != nil {
		return 0, "", err
	}
	// сохранить в кеш
	entry := steamCacheEntry{
		Price:     price,
		Raw:       raw,
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(steamCacheTTL),
	}
	steamCacheMutex.Lock()
	steamCache[marketHashName] = entry
	steamCacheMutex.Unlock()
	return price, raw, nil
}

// ====== retry wrapper ======
func fetchSteamPriceWithRetry(marketHashName string) (float64, string, error) {
	var lastErr error
	maxAttempts := 5
	baseDelay := 250 * time.Millisecond

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		price, raw, err := fetchSteamPriceOnce(marketHashName)
		if err == nil {
			return price, raw, nil
		}
		lastErr = err

		// Если получили явный 429 — даём более длинную паузу
		if strings.Contains(err.Error(), "429") {
			// экспоненциальная пауза при 429
			wait := time.Duration(800*(1<<uint(attempt-1))) * time.Millisecond
			jitter := time.Duration(rand.Intn(400)) * time.Millisecond
			time.Sleep(wait + jitter)
		} else {
			// обычный backoff + jitter
			sleep := time.Duration(int64(baseDelay) * (1 << (attempt - 1)))
			jitter := time.Duration(rand.Intn(500)+100) * time.Millisecond
			time.Sleep(sleep + jitter)
		}
	}
	return 0, "", fmt.Errorf("steam fetch failed after retries: %v", lastErr)
}

// ====== Один запрос к Steam priceoverview (через прокси-клиент, если он есть) ======
func fetchSteamPriceOnce(marketHashName string) (float64, string, error) {
	endpoint := "https://steamcommunity.com/market/priceoverview/"
	params := url.Values{}
	params.Set("currency", "5") // 5 — рубли (проверь, при необходимости подставь нужную)
	params.Set("appid", "730")
	params.Set("market_hash_name", marketHashName)

	reqURL := endpoint + "?" + params.Encode()
	req, _ := http.NewRequest("GET", reqURL, nil)
	req.Header.Set("User-Agent", userAgentSteam)
	req.Header.Set("Accept", "application/json")

	var client *http.Client
	if httpClient != nil {
		client = httpClient
	} else {
		client = httpClientSmall
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("http error: %v", err)
	}
	defer resp.Body.Close()

	// обрабатываем статус
	if resp.StatusCode == http.StatusTooManyRequests {
		return 0, "", fmt.Errorf("429")
	}
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var parsed map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, "", fmt.Errorf("json decode: %v", err)
	}

	var rawPrice string
	if v, ok := parsed["lowest_price"].(string); ok && v != "" {
		rawPrice = v
	} else if v, ok := parsed["median_price"].(string); ok && v != "" {
		rawPrice = v
	} else {
		return 0, "", fmt.Errorf("no price field in steam response")
	}

	priceFloat, err := parseSteamPriceString(rawPrice)
	if err != nil {
		return 0, rawPrice, fmt.Errorf("parse price '%s': %v", rawPrice, err)
	}
	return priceFloat, rawPrice, nil
}

// ====== Главная: обновить топ-N и пересчитать latestAnalysisResults ======
func RefreshTopLiquidAndComputeProfit(topN int) {
	if topN <= 0 {
		topN = 100
	}
	if topN > maxTopToQuery {
		topN = maxTopToQuery
	}

	log.Printf("🔎 RefreshTopLiquidAndComputeProfit: собираю топ %d по объёму...", topN)
	top := topNFromPriceMap(topN)
	sem := make(chan struct{}, steamConcurrency)
	wg := &sync.WaitGroup{}

	resultsMap := make(map[string]CombinedItem)
	resultsMutex := &sync.Mutex{}

	processed := 0
	for _, it := range top {
		wg.Add(1)
		sem <- struct{}{}
		go func(ent marketEntry) {
			defer wg.Done()
			// увеличенный рандомный sleep, чтобы пачки запросов были более "размазаны"
			time.Sleep(time.Duration(rand.Intn(200)+200) * time.Millisecond)

			priceSteam, raw, err := getSteamPriceCached(ent.Name)
			if err != nil {
				log.Printf("⚠ steam fetch err for '%s': %v", ent.Name, err)
				<-sem
				return
			}

			var profitPercent float64
			if ent.Price > 0 {
				profitPercent = (priceSteam - ent.Price) / ent.Price * 100.0
			}

			ci := CombinedItem{
				Name:          ent.Name,
				BuffID:        0,
				IconURL:       "",
				Exterior:      "",
				BuffPrice:     0.0,
				BuffSellNum:   0,
				MarketPrice:   ent.Price,
				MarketVolume:  ent.Volume,
				SteamPrice:    priceSteam,
				SteamPriceRaw: raw,
				ProfitPercent: profitPercent,
				ProfitRub:     priceSteam - ent.Price,
				Status:        "market-vs-steam",
			}

			resultsMutex.Lock()
			resultsMap[ent.Name] = ci
			resultsMutex.Unlock()

			<-sem
		}(it)
		processed++
		// Каждые 30 запросов — небольшая пауза, чтобы снизить риск пачечных 429
		if processed%30 == 0 {
			time.Sleep(120 * time.Millisecond)
		}
	}
	wg.Wait()

	var combined []CombinedItem
	for _, v := range resultsMap {
		combined = append(combined, v)
	}
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].ProfitPercent > combined[j].ProfitPercent
	})

	analysisMutex.Lock()
	latestAnalysisResults = combined
	analysisMutex.Unlock()

	log.Printf("✅ RefreshTopLiquidAndComputeProfit завершён: проверено %d предметов, <- в latestAnalysisResults %d записей", len(top), len(combined))
}
