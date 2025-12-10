# Анализ репозитория `FOSWLY/translate-backend`

## 1. Назначение и общий принцип работы

1. `translate-backend` — это HTTP-сервис (на Bun + Elysia), который предоставляет **унифицированный REST API для текстового перевода, определения языка и получения списка языков** над несколькими провайдерами, поддерживаемыми библиотекой [`@toil/translate`](https://github.com/FOSWLY/translate).
2. Сам сервер **не реализует протоколы конкретных переводчиков** (Яндекс, Bing, LibreTranslate и т.п.) — он делегирует всю логику библиотеке `@toil/translate` (через `TranslationClient`), а сам отвечает за:
   - HTTP-слой (маршруты `/v2/translate`, `/v2/detect`, `/v2/getLangs`, `/v2/health`),
   - валидацию входных данных (через `elysia` + `typebox` схемы в `src/models/*.ts`),
   - конфигурацию (env → `config.ts` → `ConfigSchema`),
   - логирование (`@vaylo/pino`), CORS и Swagger-документацию.
3. Вся сетевая активность по обращению к внешним переводческим сервисам (Yandex, MSEdge, Bing, LibreTranslate и др.) находится **внутри зависимости `@toil/translate`**, которую данный сервис использует как «чёрный ящик».

---

## 2. Структура сервиса и HTTP-маршруты

### 2.1. Точка входа (`src/index.ts`)

```ts
const app = new Elysia({ prefix: "/v2" })
  .use(swagger(...))
  .use(HttpStatusCode())
  .use(cors(config.cors))
  .error({ LIBRE_TRANSLATE_DISABLED: LibreTransalteDisabledError })
  .onError(({ set, code, error, httpStatus }) => { ... })
  .use(health)
  .use(translate)
  .use(detect)
  .use(getLangs)
  .listen({ port: config.server.port, hostname: config.server.hostname });
```

- Базовый префикс API — `/v2`.
- Подключены плагины:
  - `@elysiajs/swagger` — генерация Swagger/Scalar UI по пути `/v2/docs`;
  - `@elysiajs/cors` — CORS-конфигурация (`config.cors`);
  - `elysia-http-status-code` — удобный enum HTTP-статусов.
- Глобальный обработчик ошибок `.onError`:
  - `NOT_FOUND` → `{ detail: "Route not found :(" }`;
  - `VALIDATION` → массив ошибок валидации;
  - `LIBRE_TRANSLATE_DISABLED` → статус 403 + сообщение из `LibreTransalteDisabledError`;
  - остальные → логируются через `log.error` и возвращаются как `{ error: message }`.

### 2.2. Маршрут `/v2/health`

Файл: `src/controllers/health/index.ts`

```ts
export default new Elysia().group("/health", (app) =>
  app.get("/", () => ({
      version: config.app.version,
      status: "ok" as const,
    }),
    { response: { 200: HealthResponse }, ... }
  ),
);
```

- Возвращает простое состояние сервиса и версию из `config.app.version`.

### 2.3. Маршрут `/v2/translate`

Файл: `src/controllers/translate/index.ts`

```ts
import { TranslationParams, TranslationSuccessResponse } from "@/models/translate.model";
import { createClient } from "@/utils/client";

async function translate({ text, lang, service }: TranslationParams) {
  const client = createClient(service);
  return await client.translate(text, lang);
}

export default new Elysia().group("/translate", (app) =>
  app
    .post("/", async ({ body }) => await translate(body), {
      body: TranslationParams,
      response: { 200: TranslationSuccessResponse },
      detail: { summary: "Translate text", tags: ["Translate"] },
    })
    .get("/", async ({ query }) => await translate(query), {
      query: TranslationParams,
      ...translateSharedOpts,
    }),
);
```

- Принимает:
  - `lang`: либо код языка (`"en"`), либо пару (`"en-ru"`) — `Lang | LangPair`;
  - `text`: строка или массив строк;
  - `service` (опционально): конкретный провайдер из `ClientType.TranslationService` (`YandexBrowser`, `YandexCloud`, `MSEdge`, `Bing`, `LibreTranslate` и др.).
- Делегирует перевод в `TranslationClient.translate(text, lang)` из `@toil/translate`.

### 2.4. Маршрут `/v2/detect`

Файл: `src/controllers/detect/index.ts`

```ts
import { DetectParams, DetectSuccessResponse } from "@/models/translate.model";

async function detect({ service, text }: DetectParams) {
  const client = createClient(service);
  return await client.detect(text);
}

export default new Elysia().group("/detect", (app) =>
  app
    .post("/", async ({ body }) => await detect(body), { body: DetectParams, ... })
    .get("/", async ({ query }) => await detect(query), { query: DetectParams, ... }),
);
```

- Принимает текст и опционально `service`, возвращает:
  - `lang`: определённый язык;
  - `score`: числовую оценку уверенности или `null`.

### 2.5. Маршрут `/v2/getLangs`

Файл: `src/controllers/getLangs/index.ts`

```ts
import { GetLangsParams, GetLangsSuccessResponse } from "@/models/translate.model";

async function getLangs({ service }: GetLangsParams) {
  const client = createClient(service);
  return await client.getLangs();
}

export default new Elysia().group("/getLangs", (app) =>
  app.get("/", async ({ query }) => await getLangs(query), {
    query: GetLangsParams,
    response: { 200: GetLangsSuccessResponse },
  }),
);
```

- Возвращает список языков, который зависит от выбранного провайдера.
- Тип ответа — либо массив `Lang` (одиночные коды), либо массив `LangPair` (`"from-to"`).

---

## 3. Взаимодействие с библиотекой `@toil/translate` (внешние вызовы)

### 3.1. Клиент перевода (`src/utils/client.ts`)

```ts
import TranslationClient from "@toil/translate";
import { TranslationService } from "@toil/translate/types/client";
import config from "@/config";
import { LibreTransalteDisabledError } from "@/errors";

const { app: { allowUnsafeEval } } = config;

export function createClient(service?: TranslationService) {
  if (service === TranslationService.libretranslate && allowUnsafeEval !== true) {
    throw new LibreTransalteDisabledError();
  }

  return new TranslationClient({
    service,
    allowUnsafeEval,
  });
}
```

**Куда и как дальше идут запросы:**

- `TranslationClient` из `@toil/translate` скрывает внутреннюю реализацию сетевых запросов:
  - для **YandexBrowser/YandexCloud/YandexTranslate/YandexGPT** — библиотека сама обращается к соответствующим API Яндекса (включая приватные VOT/Cloud/GPT эндпоинты, как реализовано в `FOSWLY/translate`/`FOSWLY/vot.js`);
  - для **MSEdge/Bing** — к публичным/клиентским API Microsoft;
  - для **LibreTranslate** — к инстансу LibreTranslate (локальный или облачный).
- Этот backend **не знает конкретные URL и протоколы** каждого сервиса, а только прокидывает флаг `service` и опцию `allowUnsafeEval`.

**Зачем такой слой:**

- Отделить HTTP-API (любые клиенты/языки) от реализации переводчиков в TS-библиотеке.
- Позволить подключать новые провайдеры или менять логику работы `@toil/translate` без изменения внешнего API `/v2/*`.

### 3.2. Безопасность LibreTranslate (`LibreTransalteDisabledError`)

Файл: `src/errors.ts`

```ts
export class LibreTransalteDisabledError extends Error {
  constructor() {
    super("LibreTranslate is disabled on this server");
  }
}
```

- Если переменная окружения `ALLOW_UNSAFE_EVAL` не установлена в `true`, попытка использовать `service=libretranslate` приведёт к 403 и сообщению об отключении LibreTranslate.
- Это связано с тем, что библиотека `@toil/translate` может использовать `eval`/`Function` или аналоги для некоторых провайдеров, что нежелательно в некоторых средах; флаг позволяет явно это разрешить.

---

## 4. Конфигурация, логирование и вспомогательные части

### 4.1. Конфигурация (`src/config.ts` + `src/schemas/config.ts`)

#### Схема

Файл: `src/schemas/config.ts`

- Описывает структуру объекта конфигурации через `@sinclair/typebox` (`ConfigSchema`):
  - `server.port`, `server.hostname` — порт/хост HTTP-сервера;
  - `app.name`, `app.desc`, `app.contact_email`, `app.allowUnsafeEval`;
  - `cors` — объект для настройки CORS;
  - `logging` — настройки уровня логирования, пути до файла, интеграции с Loki (`logging.loki.*`).

#### Загрузка

Файл: `src/config.ts`

```ts
export default Value.Parse(ConfigSchema, {
  server: {
    port: Bun.env.SERVICE_PORT,
    hostname: Bun.env.SERVICE_HOST,
  },
  app: {
    name: Bun.env.APP_NAME,
    desc: Bun.env.APP_DESC,
    contact_email: Bun.env.APP_CONTACT_EMAIL,
    allowUnsafeEval: Bun.env.ALLOW_UNSAFE_EVAL === "true",
  },
  cors: {},
  logging: {
    level: Bun.env.NODE_ENV === "production" ? "info" : "debug",
    logPath: path.join(__dirname, "..", "logs"),
    logToFile: Bun.env.LOG_TO_FILE === "true",
    loki: {
      host: Bun.env.LOKI_HOST,
      user: Bun.env.LOKI_USER,
      password: Bun.env.LOKI_PASSWORD,
      label: Bun.env.LOKI_LABEL ?? "translate-backend",
    },
  },
});
```

- Все значения берутся из `.env` (см. `.example.env`) и валидируются по схеме; при несоответствии сервер не стартует.

### 4.2. Логирование (`src/logging.ts`)

```ts
import { PinoClient } from "@vaylo/pino";

const { logging: { loki, level, logPath, logToFile } } = config;

export const loggerClient = new PinoClient({ loki, level, logToFile, logPath });
export const log = loggerClient.init();
```

- Логи пишутся:
  - в stdout,
  - опционально в файл `logs/*` (если `LOG_TO_FILE=true`),
  - опционально в Loki (через HTTP) по данным в `logging.loki`.
- Это **дополнительный внешний HTTP-трафик**: если настроен Loki, `@vaylo/pino` сам шлёт батчи логов в Loki endpoint (`LOKI_HOST`), но реализация скрыта внутри этой зависимости.

### 4.3. Модели и типы (`src/models/*.ts`)

- `translate.model.ts` определяет схемы:
  - `TranslationParams` (lang, text, service),
  - `DetectParams`, `GetLangsParams`,
  - `TranslationSuccessResponse`, `DetectSuccessResponse`, `GetLangsSuccessResponse`.
- Сами модели не делают запросов, только описывают форму входа/выхода.

---

## 5. Сетевое поведение: сводка «куда, как и зачем»

### 5.1. Исходящий HTTP-трафик из самого `translate-backend`

Прямо в этом репозитории **нет кода, который сам руками ходит по HTTP** (нет `fetch`, `axios`, `undici` и т.п.) — все внешние запросы инкапсулированы в подключённых библиотеках:

1. **`@toil/translate`** — основная библиотека, которая при вызове методов `translate`, `detect`, `getLangs`:
   - ходит на соответствующие API выбранного провайдера:
     - Яндекс (Browser/Cloud/Translate/GPT),
     - Microsoft Edge/Bing,
     - LibreTranslate,
     - и др. провайдеры, поддерживаемые библиотекой;
   - учитывает лимиты, авторизацию, форматы тела и заголовков (включая сложные случаи с Yandex VOT и т.п.);
   - отдает результат в унифицированной форме.
2. **`@vaylo/pino`** — при включённой Loki-интеграции отправляет логи в Loki (`LOKI_HOST`) по HTTP.

Таким образом, **translate-backend служит чистым фасадом**, а не самостоятельно реализующим любой протокол переводчиков.

### 5.2. Входящий HTTP-трафик (API сервиса)

Сервис принимает запросы по следующим основным URL (с префиксом `/v2`):

- `GET /v2/health` — состояние сервиса;
- `GET/POST /v2/translate` — перевод текста (`TranslationParams`);
- `GET/POST /v2/detect` — определение языка (`DetectParams`);
- `GET /v2/getLangs` — список языков (`GetLangsParams`);
- `GET /v2/docs` и связанные пути — Swagger/Scalar UI.

Все ответы и ошибки типизированы и описаны в Swagger через `elysia` + `@elysiajs/swagger`.

---

## 6. Итоговые выводы

1. `translate-backend` — это **переиспользуемый, язык-независимый HTTP-фасад** над библиотекой `@toil/translate`, предназначенный для упрощения доступа к нескольким переводческим провайдерам через единый REST API.
2. Сам сервер **не знает деталей сетевых протоколов** Яндекс/Bing/LibreTranslate: он только создаёт инстанс `TranslationClient` и вызывает методы `translate/detect/getLangs`, которые дальше уже делают внешние HTTP-запросы.
3. Внешний трафик самого `translate-backend` сводится к:
   - входящим запросам клиентов (ваши приложения, внешние скрипты),
   - исходящим запросам к провайдерам перевода — через `@toil/translate`,
   - опциональным запросам к Loki (`@vaylo/pino`), если включено логирование в Loki.
4. Благодаря строгой схеме конфигурации и модели ошибок (включая явное отключение LibreTranslate через `ALLOW_UNSAFE_EVAL`) сервис можно безопасно использовать в продакшене и масштабировать, не меняя клиентов при добавлении/изменении конкретных переводчиков. 
