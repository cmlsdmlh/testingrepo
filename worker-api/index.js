// Это наш WASM-файл (мы его скомпилируем)
import wasmModule from './main.wasm';

// --- 1. Инициализация Go (WASM) ---
const go = new Go();
// 'instantiateStreaming' - самый быстрый способ загрузить WASM
const wasmInstance = WebAssembly.instantiateStreaming(
    fetch(wasmModule),
    go.importObject
);

export default {
    // --- 2. Это главный обработчик запросов ---
    async fetch(request, env, ctx) {
        // 'env.RESULTS_BUCKET' - это наш R2 бакет
        if (!env.RESULTS_BUCKET) {
            return new Response('R2 Bucket не подключен', { status: 500 });
        }

        // Ждем, пока Go (WASM) загрузится (только при первом запросе)
        go.run((await wasmInstance).instance);

        // --- 3. Получаем данные ---
        const url = new URL(request.url);

        // Получаем параметры из ?min_profit=...
        const params = {
            min_profit: url.searchParams.get('min_profit') || '0',
            min_price: url.searchParams.get('min_price') || '0',
            max_price: url.searchParams.get('max_price') || '9999999',
        };

        // ❗️ Запрос к R2 за нашим JSON-файлом
        const r2Object = await env.RESULTS_BUCKET.get('latest-data.json');
        if (r2Object === null) {
            return new Response('Данные еще не спарсились', { status: 404 });
        }

        // Получаем текст из R2
        const itemsJSON = await r2Object.text();

        // --- 4. ❗️ ВЫЗОВ GO-ФУНКЦИИ ---
        // Вызываем нашу Go-функцию 'filterItems', которую мы "экспортировали"
        const filteredJsonString = filterItems(itemsJSON, params);

        // --- 5. Отдаем ответ ---
        return new Response(filteredJsonString, {
            headers: { 'Content-Type': 'application/json' },
        });
    },
};