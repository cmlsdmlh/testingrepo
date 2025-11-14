package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io" // ❗️❗️❗️ НОВЫЙ ИМПОРТ ❗️❗️❗️
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	buffAPIBaseURL     = "https://buff.163.com/api/market/goods"
	cnyToRubRate       = 11.354
	marketCommission   = 0.1
	steamCommission    = 0.15
	liquidityThreshold = 5
	numWorkers         = 1
	proxyFile          = "proxies.txt"
)

type BuffItem struct {
	ID           int    `json:"id"`
	Name         string `json:"market_hash_name"`
	SellMinPrice string `json:"sell_min_price"`
	SellNum      int    `json:"sell_num"`
	GoodsInfo    struct {
		IconURL       string `json:"icon_url"`
		SteamPriceCNY string `json:"steam_price_cny"`
		Tags          struct {
			Exterior struct {
				LocalizedName string `json:"localized_name"`
			} `json:"exterior"`
		} `json:"info"`
	} `json:"goods_info"`
	SteamMarketURL string `json:"steam_market_url"`
}

type BuffAPIResponse struct {
	Code string `json:"code"`
	Data struct {
		Items     []BuffItem `json:"items"`
		TotalPage int        `json:"total_page"`
	} `json:"data"`
}

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

// --- Глобальные переменные ---
// (Тут ничего не меняется)
var proxyList []string
var proxyListMutex = &sync.RWMutex{}
var httpClient *http.Client
var latestAnalysisResults []CombinedItem
var analysisMutex = &sync.RWMutex{}

// --- Функции парсера и HTTP ---

// (loadProxies - БЕЗ ИЗМЕНЕНИЙ)
func loadProxies() {
	file, err := os.Open(proxyFile)
	if err != nil {
		log.Fatalf("❌ Крах: Не могу открыть файл с прокси '%s': %v", proxyFile, err)
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	proxyListMutex.Lock()
	proxyList = lines
	proxyListMutex.Unlock()
	if len(proxyList) == 0 {
		log.Fatalln("❌ Крах: Файл `proxies.txt` найден, но он пустой.")
	}
	log.Printf("✅ Загружено %d прокси из файла.", len(proxyList))
}

// (getProxy - БЕЗ ИЗМЕНЕНИЙ)
func getProxy() (*url.URL, error) {
	proxyListMutex.RLock()
	if len(proxyList) == 0 {
		proxyListMutex.RUnlock()
		return nil, fmt.Errorf("список прокси пуст")
	}
	randomProxyStr := proxyList[rand.Intn(len(proxyList))]
	proxyListMutex.RUnlock()
	proxyURL, err := url.Parse(randomProxyStr)
	if err != nil {
		log.Printf("⚠️ Ошибка парсинга URL прокси: %v", err)
		return nil, err
	}
	return proxyURL, nil
}

// (createHTTPClient - БЕЗ ИЗМЕНЕНИЙ)
func createHTTPClient() {
	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			host := req.URL.Host // Получаем хост

			// ⭐ НОВАЯ ЛОГИКА ⭐
			// Проверяем и Buff, И Steam
			if strings.Contains(host, "buff.163.com") || strings.Contains(host, "steamcommunity.com") {
				// log.Printf("DEBUG: Использую прокси для %s", host) // (можно добавить для отладки)
				return getProxy()
			}
			return nil, nil
		},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	httpClient = &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
	}
	log.Println("✅ HTTP-клиент с поддержкой ротации прокси (для Buff и Steam) создан.")
}

// ❗️❗️❗️ ВОТ ТУТ ГЛАВНЫЕ ИЗМЕНЕНИЯ ❗️❗️❗️
// fetchBuffPage - теперь с отладкой
func fetchBuffPage(page int) (BuffAPIResponse, error) {
	var buffResponse BuffAPIResponse
	pageURL := fmt.Sprintf("%s?game=csgo&page_num=%d&page_size=80", buffAPIBaseURL, page)

	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return buffResponse, fmt.Errorf("ошибка создания запроса (стр %d): %v", page, err)
	}
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 YaBrowser/25.10.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("sec-fetch-site", "none")
	resp, err := httpClient.Do(req)
	if err != nil {
		return buffResponse, fmt.Errorf("ошибка запроса через прокси (стр %d): %v", page, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return buffResponse, fmt.Errorf("ошибка чтения тела ответа (стр %d): %v", page, err)
	}

	if err := json.Unmarshal(bodyBytes, &buffResponse); err != nil {
		contentType := resp.Header.Get("Content-Type")
		log.Printf("❌ ОШИБКА ПАРСИНГА JSON (стр %d): %v", page, err)
		log.Printf("ℹ️ Content-Type ответа: %s", contentType)

		// Превращаем тело ответа в строку, чтобы посмотреть
		bodyString := string(bodyBytes)
		logLength := 500
		if len(bodyString) < logLength {
			logLength = len(bodyString)
		}

		// Возвращаем ошибку, чтобы цикл прервался
		return buffResponse, fmt.Errorf("ошибка парсинга JSON, тело ответа - не JSON (см. лог)")
	}

	if buffResponse.Code != "OK" || buffResponse.Data.Items == nil {
		// Это "правильный" JSON, но с ошибкой от API
		return buffResponse, fmt.Errorf("API Buff вернул ошибку (стр %d): %s", page, buffResponse.Code)
	}

	return buffResponse, nil
}

// (Новый код с ПАУЗОЙ)
func worker(id int, wg *sync.WaitGroup, jobs <-chan int, results chan<- []BuffItem) {
	defer wg.Done()

	for page := range jobs {
		log.Printf("[Воркер %d] Начинаю страницу %d...", id, page)

		buffResponse, err := fetchBuffPage(page)
		if err != nil {
			log.Printf("❌ [Воркер %d/Стр %d] %v", id, page, err)
			// Все равно ждем, даже если ошибка, чтобы не "долбить"
			time.Sleep(time.Duration(rand.Intn(50000)+60000) * time.Millisecond)
			continue
		}

		results <- buffResponse.Data.Items
		log.Printf("✅ [Воркер %d] Закончил страницу %d, найдено %d предметов.", id, page, len(buffResponse.Data.Items))

		time.Sleep(time.Duration(rand.Intn(50000)+60000) * time.Millisecond)
	}
}

// (analyzeResults - БЕЗ ИЗМЕНЕНИЙ)
func analyzeResults(allBuffItems []BuffItem) {
	log.Println("🧠 Начинаю анализ и расчет профита...")
	priceMapMutex.RLock()
	defer priceMapMutex.RUnlock()
	if len(priceMap) == 0 {
		log.Println("⚠️ Внимание: Анализ запущен, но priceMap (цены Market) пуст.")
	}
	var combinedResults []CombinedItem
	for _, buffItem := range allBuffItems {
		itemName := buffItem.Name
		buffPriceRUB := parseFloat(buffItem.SellMinPrice) * cnyToRubRate
		steamPriceRUB := parseFloat(buffItem.GoodsInfo.SteamPriceCNY) * cnyToRubRate
		if itemName == "" || buffPriceRUB == 0 {
			continue
		}
		marketData, foundOnMarket := priceMap[itemName]
		marketPrice := 0.0
		marketVolume := 0
		profitPercent := -999.0
		profitRub := 0.0
		status := "loss"
		profitSteamPercent := -999.0
		statusSteam := "loss"
		if foundOnMarket {
			marketPrice = marketData.Price
			marketVolume = marketData.Volume
			if marketVolume < liquidityThreshold {
				profitPercent = -998.0
				status = "illiquid"
			} else {
				netMarketPrice := marketPrice * (1 - marketCommission)
				profitRub = netMarketPrice - buffPriceRUB
				profitPercent = (profitRub / buffPriceRUB) * 100
				if profitPercent > 2 {
					status = "profit"
				}
			}
		}
		if steamPriceRUB > 0 {
			netSteamPrice := steamPriceRUB * (1 - steamCommission)
			profitSteamRub := netSteamPrice - buffPriceRUB
			profitSteamPercent = (profitSteamRub / buffPriceRUB) * 100
			if profitSteamPercent > 2 {
				statusSteam = "profit"
			}
		}
		combinedResults = append(combinedResults, CombinedItem{
			Name:               itemName,
			BuffID:             buffItem.ID,
			IconURL:            buffItem.GoodsInfo.IconURL,
			Exterior:           buffItem.GoodsInfo.Tags.Exterior.LocalizedName,
			BuffPrice:          buffPriceRUB,
			BuffSellNum:        buffItem.SellNum,
			MarketPrice:        marketPrice,
			MarketVolume:       marketVolume,
			SteamPrice:         steamPriceRUB,
			SteamMarketURL:     buffItem.SteamMarketURL,
			ProfitPercent:      profitPercent,
			ProfitRub:          profitRub,
			Status:             status,
			ProfitSteamPercent: profitSteamPercent,
			StatusSteam:        statusSteam,
		})
	}
	analysisMutex.Lock()
	latestAnalysisResults = combinedResults
	analysisMutex.Unlock()
	log.Printf("✅ Анализ завершен! Рассчитано %d предметов.", len(latestAnalysisResults))
}

// (ParseAndAnalyzeBuff - БЕЗ ИЗМЕНЕНИЙ)
func ParseAndAnalyzeBuff() {
	log.Println("🔥 Запускаю полный парсинг Buff...")
	startTime := time.Now()
	log.Println("🧐 Узнаю общее кол-во страниц (запрос стр. 1)...")
	pageOneResponse, err := fetchBuffPage(1)
	if err != nil {
		log.Printf("❌ Крах! Не могу получить страницу 1. Ошибка: %v", err)
		log.Println("--- (Цикл анализа прерван) ---")
		return
	}
	dynamicTotalPages := pageOneResponse.Data.TotalPage
	log.Printf("ℹ️ Всего страниц на Buff: %d", dynamicTotalPages)
	jobs := make(chan int, dynamicTotalPages)
	results := make(chan []BuffItem, dynamicTotalPages)
	results <- pageOneResponse.Data.Items
	var wg sync.WaitGroup
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go worker(w, &wg, jobs, results)
	}
	log.Printf("👨‍💻 %d воркеров запущены и ждут работы...", numWorkers)
	for j := 2; j <= 2; {
		jobs <- j
		j += 1
	}
	close(jobs)
	wg.Wait()
	close(results)
	log.Println("🏁 Все воркеры закончили работу.")
	var allBuffItems []BuffItem
	for pageItems := range results {
		allBuffItems = append(allBuffItems, pageItems...)
	}
	log.Printf("📊 Собрано %d предметов со всех страниц.", len(allBuffItems))
	analyzeResults(allBuffItems)
	duration := time.Since(startTime)
	log.Printf("✅ Полный цикл (Парсинг + Анализ) завершен за %s", duration)
}

// (parseFloat - БЕЗ ИЗМЕНЕНИЙ)
func parseFloat(str string) float64 {
	val, _ := strconv.ParseFloat(str, 64)
	return val
}
