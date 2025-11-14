import wasmModule from './main.wasm';

// --- 1. Инициализация Go (WASM) ---
const go = new Go();
const wasmInstance = WebAssembly.instantiateStreaming(
    fetch(wasmModule),
    go.importObject
);

// ❗️❗️❗️ НАШЕ НОВОЕ ХРАНИЛИЩЕ ВМЕСТО R2 ❗️❗️❗️
// Это глобальная переменная, которая будет хранить JSON
let latestData = null;
let isParsing = false; // Флаг, чтобы не запускать 10 парсеров сразу

/**
 * Запускает парсер, сохраняет результат в latestData
 * @returns {Promise<string>} - JSON-строка с данными
 */
async function runParser() {
    if (isParsing) {
        console.log("Парсинг уже запущен, ждем...");
        return "Парсинг уже запущен";
    }
    isParsing = true;
    console.log("Запускаю Go-парсер (runAnalysis)...");

    try {
        // Ждем, пока WASM загрузится
        go.run((await wasmInstance).instance);

        // Вызываем Go-функцию 'runAnalysis'
        const jsonData = await runAnalysis(); // Эта функция "экспортирована" из Go

        if (jsonData && jsonData.length > 0) {
            latestData = jsonData; // ❗️ Сохраняем результат в нашу "память"
            console.log(`Парсинг завершен. Сохранено ${latestData.length} байт.`);
        } else {
            console.error("Парсер Go вернул пустой результат.");
        }
        return latestData;
    } catch (e) {
        console.error("Ошибка при выполнении Go-парсера:", e);
    } finally {
        isParsing = false; // Снимаем флаг
    }
}

export default {
    /**
     * ❗️ ОБРАБОТЧИК API (ТРИГГЕР FETCH)
     */
    async fetch(request, env, ctx) {
        let dataToFilter = latestData;

        // ❗️ Если в памяти пусто (холодный старт),
        // запускаем парсер ПРЯМО СЕЙЧАС.
        if (dataToFilter === null) {
            console.warn("Холодный старт! Запускаю парсер по запросу...");
            // Это будет долгий ответ для первого пользователя
            dataToFilter = await runParser();
            if (dataToFilter === null) {
                return new Response('Данные еще не спарсились, попробуйте через 2 минуты', { status: 503 });
            }
        }

        // Ждем, пока WASM загрузится (если еще не)
        go.run((await wasmInstance).instance);

        // --- Получаем параметры из ?min_profit=... ---
        const url = new URL(request.url);
        const params = {
            min_profit: url.searchParams.get('min_profit') || '0',
            min_price: url.searchParams.get('min_price') || '0',
            max_price: url.searchParams.get('max_price') || '9999999',
        };

        // ❗️ Вызываем Go-функцию 'filterItems'
        const filteredJsonString = filterItems(dataToFilter, params);

        return new Response(filteredJsonString, {
            headers: { 'Content-Type': 'application/json' },
        });
    },

    /**
     * ❗️ ОБРАБОТЧИК ПАРСЕРА (ТРИГГЕР CRON)
     */
    async scheduled(event, env, ctx) {
        console.log("Запускаю плановый парсинг по Cron...");
        // Просто запускаем парсер. Он сам сохранит результат в 'latestData'.
        ctx.waitUntil(runParser());
    }
};