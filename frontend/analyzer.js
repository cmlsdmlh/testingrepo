// analyzer.js (vServer 1.0 - Загрузка из Go API)

// --- НАСТРОЙКИ ---
// (Они теперь в Go, но для калькулятора оставим)
const MARKET_COMMISSION_RATE = 0.1;
// --- /НАСТРОЙКИ ---

let allResults = [];
let currentSort = {
    key: 'profitPercent',
    direction: 'desc'
};

document.addEventListener('DOMContentLoaded', () => {
    // --- Элементы Управления ---
    const reloadButton = document.getElementById('reload-btn');
    const statusText = document.getElementById('status-text');

    // --- Калькулятор ---
    const calcBuyInput = document.getElementById('calc-buy-price');
    const calcSellInput = document.getElementById('calc-sell-price');
    calcBuyInput.addEventListener('input', updateCalculator);
    calcSellInput.addEventListener('input', updateCalculator);

    // --- Назначаем действия ---
    reloadButton.addEventListener('click', fetchDataFromServer);

    // "Слушаем" заголовки для сортировки
    document.querySelectorAll('th.sortable').forEach(th => {
        th.addEventListener('click', () => {
            const sortKey = th.dataset.sortKey;
            if (currentSort.key === sortKey) {
                currentSort.direction = (currentSort.direction === 'desc') ? 'asc' : 'desc';
            } else {
                currentSort.key = sortKey;
                currentSort.direction = 'desc';
            }
            updateSortHeaders();
            renderTable();
        });
    });

    // Загружаем данные при открытии
    fetchDataFromServer();
});

// ❗️❗️❗️ НОВАЯ ГЛАВНАЯ ФУНКЦИЯ ❗️❗️❗️
async function fetchDataFromServer() {
    const tableBody = document.getElementById('results-table');
    const reloadButton = document.getElementById('reload-btn');
    const statusText = document.getElementById('status-text');

    reloadButton.textContent = 'Загрузка...';
    reloadButton.disabled = true;
    statusText.textContent = 'Запрашиваю данные с Go-сервера...';
    tableBody.innerHTML = `<tr><td colspan="6">Загружаю данные...</td></tr>`;

    try {
        // ❗️❗️❗️ ВОТ ИЗМЕНЕНИЯ ❗️❗️❗️

        // 1. Читаем ВСЕ три фильтра
        const minProfit = document.getElementById('profit-filter-input').value || 0;
        const minPrice = document.getElementById('min-price-input').value || 0;
        // (Для maxPrice ставим "бесконечность", если поле пустое)
        const maxPrice = document.getElementById('max-price-input').value || 9999999;

        // 2. Строим URL со всеми параметрами
        const fetchURL = `/api/items?min_profit=${minProfit}&min_price=${minPrice}&max_price=${maxPrice}`;

        // (Эта строчка уже была, но теперь она покажет нам новый URL - удобно!)
        statusText.textContent = `Запрашиваю ${fetchURL}...`;

        // 3. Делаем запрос по новому URL
        const response = await fetch(fetchURL);

        if (!response.ok) {
            throw new Error(`Ошибка сервера: ${response.statusText}`);
        }

        const data = await response.json();

        if (!data) {
            // Сервер мог отдать null, если анализ еще не прошел
            allResults = [];
            statusText.textContent = 'Сервер еще не провел первый анализ. Подождите 1-2 мин и обновите.';
        } else {
            allResults = data;
            statusText.textContent = `Данные загружены! Найдено ${allResults.length} предметов.`;
        }

        // Сортируем и рендерим (эти функции не изменились)
        currentSort.key = 'profitPercent';
        currentSort.direction = 'desc';
        updateSortHeaders();
        renderTable();

    } catch (error) {
        console.error('Ошибка в analyzer.js:', error);
        tableBody.innerHTML = `<tr><td colspan."6">Произошла ошибка: ${error.message}</td></tr>`;
        statusText.textContent = `Ошибка: ${error.message}`;
    } finally {
        reloadButton.textContent = 'Обновить';
        reloadButton.disabled = false;
    }
}


// (Эта функция без изменений)
function updateSortHeaders() {
    document.querySelectorAll('th.sortable').forEach(th => {
        th.classList.remove('sort-asc', 'sort-desc');
        if (th.dataset.sortKey === currentSort.key) {
            th.classList.add(currentSort.direction === 'desc' ? 'sort-desc' : 'sort-asc');
        }
    });
}

// (Эта функция почти без изменений, только JSON-поля называются чуть иначе)
function renderTable() {
    const tableBody = document.getElementById('results-table');

    // Сортировка
    const key = currentSort.key;
    const dir = currentSort.direction;
    allResults.sort((a, b) => {
        let valA = a[key];
        let valB = b[key];
        if (dir === 'asc') {
            return valA - valB;
        } else {
            return valB - valA;
        }
    });

    tableBody.innerHTML = '';
    if (allResults.length === 0) {
        tableBody.innerHTML = `<tr><td colspan="6">Пока нет данных. Сервер может еще не завершил первый анализ.</td></tr>`;
        return;
    }

    allResults.forEach(item => {
        // (Логика та же, названия полей из Go-структуры `CombinedItem`)
        let profitText = `${item.profitPercent.toFixed(1)}%`;
        let profitRubText = `${item.profitRub.toFixed(2)} ₽`;

        if (item.profitPercent === -999) {
            profitText = 'Нет на Market';
            profitRubText = '---';
        }
        if (item.profitPercent === -998) {
            profitText = `(Мало: ${item.marketVolume} шт.)`;
            profitRubText = '---';
        }
        let profitSteamText = `${item.profitSteamPercent.toFixed(1)}%`;
        if (item.profitSteamPercent === -999) profitSteamText = 'N/A';

        const marketURL = `https://market.csgo.com/?search=${encodeURIComponent(item.name)}`;
        const buffURL = `https://buff.163.com/goods/${item.buff_id}`;
        // ❗️ 'steam_market_url' теперь приходит из Go
        const steamURL = item.steam_market_url;

        const row = `
      <tr>
        <td>
          <div class="item-cell">
            <img src="${item.icon_url}" alt="">
            <div class="item-name">
              <a href="${buffURL}" target="_blank" class="name-link">${item.name}</a>
              <span class="exterior">${item.exterior}</span>
            </div>
          </div>
        </td>
        <td>
          <div class="price-cell">
            <a href="${buffURL}" target="_blank">
              <span class="platform-icon icon-buff">B</span>
              <span class="price-text">${item.buffPrice.toFixed(2)} ₽</span>
            </a>
            <span class="count">${item.buffSellNum} шт.</span>
          </div>
        </td>
        <td>
          <div class="price-cell">
            <a href="${marketURL}" target="_blank">
              <span class="platform-icon icon-market">M</span>
              <span class="price-text">${item.marketPrice.toFixed(2)} ₽</span>
            </a>
            <span class="count">${item.marketVolume} шт.</span>
          </div>
        </td>
        <td>
          <div class="price-cell">
            <a href="${steamURL}" target="_blank">
              <span class="platform-icon icon-steam"></span>
              <span class="price-text">${item.steamPrice.toFixed(2)} ₽</span>
            </a>
            <span class="count">(N/A шт.)</span>
          </div>
        </td>
        <td>
          <div class="profit-cell">
            <span class="profit-percent ${item.status}">${profitText}</span>
            <span class="profit-rub ${item.status}">${profitRubText}</span>
          </div>
        </td>
        <td>
          <div class="profit-cell">
            <span class="profit-percent ${item.statusSteam}">${profitSteamText}</span>
          </div>
        </td>
      </tr>
    `;
        tableBody.innerHTML += row;
    });
}

// (Эта функция без изменений)
function updateCalculator() {
    const buyPrice = parseFloat(document.getElementById('calc-buy-price').value) || 0;
    const sellPrice = parseFloat(document.getElementById('calc-sell-price').value) || 0;
    const resultEl = document.getElementById('calc-result');

    if (buyPrice === 0 || sellPrice === 0) {
        resultEl.innerHTML = '';
        return;
    }

    const netSellPrice = sellPrice * (1 - MARKET_COMMISSION_RATE);
    const profitAmount = netSellPrice - buyPrice;
    const profitPercent = (profitAmount / buyPrice) * 100;

    const resultHTML = `
    Чистый профит: 
    <span class="${profitAmount > 0 ? 'profit' : 'loss'}">
      ${profitAmount.toFixed(2)} ₽ (${profitPercent.toFixed(1)}%)
    </span>
  `;
    resultEl.innerHTML = resultHTML;
}