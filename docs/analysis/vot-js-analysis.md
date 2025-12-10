# Анализ репозитория `FOSWLY/vot.js`

## 1. Назначение и общий принцип работы

1. `vot.js` — это многопакетная TypeScript-библиотека для **неофициального взаимодействия с Яндекс-сервисом видеоперевода (VOT)** и сопутствующим бекендом FOSWLY VOT Backend.
2. Основная логика работы с Yandex VOT API реализована в пакете `@vot.js/core`:
   - `packages/core/src/client.ts` — высокоуровневый клиент `VOTClient` / `VOTWorkerClient`, реализующий методы: перевод видео, загрузка аудио/частей аудио, субтитры, перевод стримов, кеш перевода, создание сессий;
   - `packages/core/src/protobuf.ts` + `packages/shared/src/protos/yandex.proto` — структуры protobuf-запросов/ответов Яндекса и функции кодирования/декодирования;
   - `@vot.js/shared` (пакет `packages/shared`) предоставляет общие типы, конфигурацию, протоколы, криптографию (`secure.ts`), утилиты (`fetchWithTimeout`, `getTimestamp`).
3. `@vot.js/ext` и `@vot.js/node` — тонкие обёртки над `@vot.js/core` для браузерных userscript-окружений (добавляют security-заголовки браузера) и Node.js (используют кастомный агент `undici` через `VOTAgent`), сами сетевую логику не меняют.

---

## 2. Куда и как отправляются сетевые запросы

### 2.1. Основные хосты и пути

Клиент оперирует **двумя** основными типами бекендов:

1. **Нативное API Яндекс.Видеоперевода** (далее: «Яндекс API»):
   - Домены и пути задаются через `@vot.js/shared/config` (не в этом выводе, но по контексту из других проектов: обычно `https://api.browser.yandex.ru`).
   - Пути внутри `VOTClient.paths` (`packages/core/src/client.ts`):
     - `/video-translation/translate` — запрос перевода видео (получение статуса/URL аудио);
     - `/video-translation/audio` — загрузка оригинального аудио (целиком или чанками);
     - `/video-translation/cache` — запрос статуса кеша перевода;
     - `/video-subtitles/get-subtitles` — запрос субтитров;
     - `/stream-translation/ping-stream` — ping-keepalive для перевода стримов;
     - `/stream-translation/translate-stream` — запрос перевода стрима;
     - `/session/create` — создание криптографической сессии для подписи запросов.

2. **VOT Backend API (FOSWLY backend)**:
   - Хост `config.hostVOT` (например, `https://vot.toil.cc/v1`, как видно в других репозиториях автора).
   - В `VOTClient.paths.videoTranslation` и `videoSubtitles` **тот же путь**, но запрос идёт уже на `hostVOT` через `requestVOT` (JSON), когда `isCustomLink(url)` возвращает `true` (HLS, MPD, Epic Games CDN и т.д.).

### 2.2. Базовый HTTP-клиент и схема запросов

Файл: `packages/core/src/client.ts`, класс `MinimalClient`.

#### Базовые поля

```ts
host: string;       // host Яндекс API или воркера
schema: "http" | "https";
fetch: FetchFunction; // по умолчанию fetchWithTimeout из @vot.js/shared/utils
fetchOpts: Record<string, unknown>;
headers = {
  "User-Agent": config.userAgent,
  Accept: "application/x-protobuf",
  "Accept-Language": "en",
  "Content-Type": "application/x-protobuf",
  Pragma: "no-cache",
  "Cache-Control": "no-cache",
};
```

#### Метод `request`

```ts
async request<T = ArrayBuffer>(
  path: string,
  body: Uint8Array,
  headers: Record<string, string> = {},
  method = "POST",
): Promise<ClientResponse<T>> {
  const options = this.getOpts(new Blob([body as BlobPart]), headers, method);
  const res = await this.fetch(`${this.schema}://${this.host}${path}`, options);
  const data = (await res.arrayBuffer()) as T;
  return { success: res.status === 200, data };
}
```

- **Куда:** на URL вида `https://<host><path>`;
- **Как:**
  - тело — бинарный protobuf (Blob/Uint8Array);
  - заголовки — смесь дефолтных и переданных при вызове (в т.ч. криптографические `Vtrans-Signature`, сигнатуры сессий и т.д.);
  - сессии (`this.sessions`) кэшируются в памяти на основании `expires` и текущего времени (`getTimestamp`).

#### Метод `requestJSON`

Аналогичен, но использует `Content-Type: application/json` и сериализует тело через `JSON.stringify`.

**Использование:** `VOTClient.requestVOT` (VOT Backend), `requestVtransFailAudio` и др.

### 2.3. Создание сессий для Яндекс API (`/session/create`)

Файл: `packages/core/src/client.ts`, класс `MinimalClient`.

```ts
async getSession(module: SessionModule): Promise<ClientSession> { ... }

async createSession(module: SessionModule) {
  const uuid = getUUID();
  const body = YandexSessionProtobuf.encodeSessionRequest(uuid, module);
  const res = await this.request("/session/create", body, {
    "Vtrans-Signature": await getSignature(body),
  });
  const sessionResponse = YandexSessionProtobuf.decodeSessionResponse(res.data);
  return { ...sessionResponse, uuid };
}
```

- **Куда:** `POST https://<Yandex host>/session/create` с protobuf-телом `YandexSessionRequest`.
- **Как:**
  - поле `module` (например, `"video_translation"` или `"summarization"`) указывает тип сессии;
  - заголовок `Vtrans-Signature` содержит HMAC-подпись тела (см. `@vot.js/shared/secure`).
- **Зачем:** получить `secretKey` и `expires`, используемые для последующей генерации защищённых заголовков (`Sec-<Module>-Token`, подписи path+body и т.п. — логика в `getSecYaHeaders`, `getSignature`).

### 2.4. Запрос перевода видео через Яндекс (`/video-translation/translate`)

Файл: `packages/core/src/client.ts`, метод `translateVideoYAImpl`.

Схема:

1. Вычисление длительности и URL:
   - `const { url, duration = config.defaultDuration } = videoData;`
   - Внешние проекты (например, `vot-cli-live`) передают реальную длительность; здесь библиотека не занимается её вычислением.
2. Получение/создание сессии: `const session = await this.getSession("video-translation");`.
3. Формирование protobuf-тела:

   ```ts
   const body = YandexVOTProtobuf.encodeTranslationRequest(
     url,
     duration,
     requestLang,
     responseLang,
     translationHelp,
     extraOpts,
   );
   ```

4. Формирование заголовков и отправка:

   ```ts
   const path = this.paths.videoTranslation; // "/video-translation/translate"
   const vtransHeaders = await getSecYaHeaders("Vtrans", session, body, path);
   const apiTokenHeader = extraOpts.useLivelyVoice ? this.apiTokenHeader : {};

   const res = await this.request(path, body, {
     ...vtransHeaders,
     ...apiTokenHeader,
     ...headers,
   });
   ```

   - `getSecYaHeaders("Vtrans", ...)` строит набор заголовков вида:
     - `Sec-Vtrans-Token`, `Sec-Vtrans-Signature` и т.п. на основе `secretKey`, `uuid` и тела запроса;
   - если `extraOpts.useLivelyVoice === true`, добавляется OAuth-токен пользователя (живые голоса требуют авторизации).

5. Обработка ответа:

   ```ts
   const translationData = YandexVOTProtobuf.decodeTranslationResponse(res.data);
   const { status, translationId } = translationData;

   switch (status) {
     case VideoTranslationStatus.FINISHED:
     case VideoTranslationStatus.PART_CONTENT:
       if (!translationData.url) throw new VOTJSError("Audio link wasn't received...");
       return { translationId, translated: true, url: translationData.url, status, remainingTime: translationData.remainingTime ?? -1 };
     case VideoTranslationStatus.WAITING:
     case VideoTranslationStatus.LONG_WAITING:
       return { translationId, translated: false, status, remainingTime: translationData.remainingTime! };
     case VideoTranslationStatus.AUDIO_REQUESTED:
       // отдельная ветка для YouTube: вызывает requestVtransFailAudio + requestVtransAudio, затем повторяет запрос
     case VideoTranslationStatus.FAILED:
     case VideoTranslationStatus.SESSION_REQUIRED:
       // ошибка или требование авторизации
   }
   ```

**Зачем:**

- Запросить у Яндекса статус перевода, URL переведённого аудио и др. метаданные, поддерживая сложные сценарии (ожидание, частичный контент, необходимость загрузки оригинального аудио, живые голоса).

### 2.5. Загрузка аудио на Яндекс (`/video-translation/audio`)

Файл: `packages/core/src/client.ts`, метод `requestVtransAudio`.

1. Определение, с чем работаем — целиком аудио или частичный чанк:

   ```ts
   const body = YandexVOTProtobuf.isPartialAudioBuffer(audioBuffer)
     ? YandexVOTProtobuf.encodeTranslationAudioRequest(url, translationId, audioBuffer, partialAudio!)
     : YandexVOTProtobuf.encodeTranslationAudioRequest(url, translationId, audioBuffer, undefined);
   ```

2. Заголовки и запрос:

   ```ts
   const path = this.paths.videoTranslationAudio; // "/video-translation/audio"
   const vtransHeaders = await getSecYaHeaders("Vtrans", session, body, path);
   const res = await this.request(path, body, { ...vtransHeaders, ...headers }, "PUT");
   ```

3. Ответ декодируется в `VideoTranslationAudioResponse` (см. protobuf-структуры ниже) и возвращается.

**Зачем:**

- Позволить клиентскому коду (например, браузерному расширению) загружать оригинальное аудио видео (целиком или частями) на стороны Яндекса, когда сервис требует это для перевода новых/неизвестных видео.

### 2.6. Запрос кеша перевода (`/video-translation/cache`)

Файл: `translateVideoCache`.

- Формируется `VideoTranslationCacheRequest` и отправляется на `paths.videoTranslationCache` с теми же заголовками `Vtrans`.
- Ответ декодируется как `VideoTranslationCacheResponse` и даёт информацию о статусе кеша (стандартный/"cloning").

**Зачем:**

- Позволить быстро понять, есть ли уже готовый перевод видео без запуска реального процесса перевода.

### 2.7. Субтитры (`/video-subtitles/get-subtitles`)

Файл: `getSubtitlesYAImpl`.

- Аналогично переводу:
  - `SubtitlesRequest` кодируется, подписывается через `getSecYaHeaders("Vsubs", ...)` и отправляется на `paths.videoSubtitles`;
  - ответ декодируется как `SubtitlesResponse`, и из него извлекается массив субтитров (оригинальные и переведённые URL).

**Зачем:**

- Получение субтитров от Яндекс-сервиса, в том числе переведённых версий.

### 2.8. Перевод стримов (`/stream-translation/*`)

Файл: `translateStream` и `pingStream`.

- `StreamPingRequest` → `/stream-translation/ping-stream` — ping для поддержания сессии;
- `StreamTranslationRequest` → `/stream-translation/translate-stream` — запрос перевода стрима;
- ответ `StreamTranslationResponse` содержит `interval`:
  - `NO_CONNECTION` (0) — нет связи/стрим завершён;
  - `TRANSLATING` (10) — перевод в процессе, нужно подождать;
  - `STREAMING` (20) — вернулся объект `translatedInfo` с URL HLS-потока перевода и timestamp.

**Зачем:**

- Поддержка живого видеоперевода для стриминговых источников.

### 2.9. Взаимодействие с VOT Backend API

Файл: `requestVOT` и `translateVideoVOTImpl`, `getSubtitlesVOTImpl`.

- Если `isCustomLink(url)` возвращает `true` (HLS, MPD, специфические сервисы типа Epic Games CDN), перевод/субтитры запрашиваются **не** напрямую у Яндекса, а через VOT Backend:

  ```ts
  const res = await this.requestVOT<TranslationResponse>(
    this.paths.videoTranslation,
    {
      provider,
      service: votData.service,
      video_id: votData.videoId,
      from_lang: requestLang,
      to_lang: responseLang,
      raw_video: url,
    },
  );
  ```

- Ответ `TranslationResponse` имеет поля `status: "failed" | "success" | "waiting"`, `translated_url`, `remaining_time`, `message` и др.
- Для субтитров — аналогично, с массивом `SubtitleItem` (`lang`, `subtitle_url`, `lang_from`, и т.п.), который затем сводится к структуре аналогичной яндексовской (`language`, `url`, `translatedLanguage`, `translatedUrl`).

**Зачем:**

- Обслуживать случаи, которые Яндекс Видеоперевод не поддерживает напрямую (например, нестандартные форматы/сервисы), причём VOT Backend выступает прослойкой, сам общаясь с Яндексом или другими провайдерами.

---

## 3. Структура protobuf (`packages/shared/src/protos/yandex.proto`)

Ниже приведена ключевая часть протокола (полностью файл ~6КБ, здесь — с комментариями).

### 3.1. VideoTranslationHelpObject

```proto
message VideoTranslationHelpObject {
  // "video_file_url" or "subtitles_file_url"
  string target = 1;
  // raw url to video file or subs
  string targetUrl = 2;
}
```

- Используется в `VideoTranslationRequest.translationHelp` для передачи Яндексу прямых URL на видео/субтитры (например, для Coursera/Udemy и подобных источников).

### 3.2. VideoTranslationRequest

```proto
message VideoTranslationRequest {
  string url = 3;                 // исходный URL видео
  optional string deviceId = 4;   // mobile only, не используется библиотекой
  bool firstRequest = 5;          // первый запрос или повторный
  double duration = 6;            // длительность видео (сек)
  int32 unknown0 = 7;             // 1
  string language = 8;            // исходный язык
  bool forceSourceLang = 9;       // автоопределять или принудительно использовать language
  int32 unknown1 = 10;            // 0
  repeated VideoTranslationHelpObject translationHelp = 11; // доп. подсказки (URL файлов)
  bool wasStream = 13;            // если это было завершённое стрим-видео
  string responseLanguage = 14;   // целевой язык перевода
  int32 unknown2 = 15;            // 1
  int32 unknown3 = 16;            // до 04.2025: 1, сейчас: 2
  bool bypassCache = 17;          // игнорировать кеш (опасно, может заддосить API)
  bool useLivelyVoice = 18;       // использовать живые голоса (дорогой и привилегированный режим)
  string videoTitle = 19;         // (пока не заполняется, но поле есть)
}
```

### 3.3. VideoTranslationResponse

```proto
message VideoTranslationResponse {
  optional string url = 1;        // URL аудио перевода (если перевод готов)
  optional double duration = 2;   // длительность аудио

  int32 status = 4;               // см. enum VideoTranslationStatus в коде
  optional int32 remainingTime = 5; // секунд до завершения перевода
  optional int32 unknown0 = 6;    // 0 -> 10 при повторных запросах ожидания
  string translationId = 7;       // ID перевода
  optional string language = 8;   // детектированный язык
  optional string message = 9;    // текстовое сообщение/ошибка
  bool isLivelyVoice = 10;        // true, если результат с живыми голосами
  optional int32 unknown2 = 11;
  optional int32 shouldRetry = 12;// флаг ретрая при сбое аудио
  optional int32 unknown3 = 13;   // 1, если classic voices и есть url
}
```

- В `VOTClient` поле `status` мапится на enum `VideoTranslationStatus`.
- `translationId` критически важен для следующих запросов (загрузка аудио чанков, кеш, повторные запросы).

### 3.4. VideoTranslationCacheRequest/Response

```proto
message VideoTranslationCacheItem {
  int32 status = 1;
  optional int32 remainingTime = 2;
  optional string message = 3;
  optional int32 unknown0 = 4;
}

message VideoTranslationCacheRequest {
  string url = 1;
  double duration = 2;
  string language = 3;          // from_lang
  string responseLanguage = 4;  // to_lang
}

message VideoTranslationCacheResponse {
  VideoTranslationCacheItem default = 1;
  VideoTranslationCacheItem cloning = 2;
}
```

- Даёт информацию о текущем состоянии кеша перевода (основной и "cloning" варианты).

### 3.5. Загрузка аудио: AudioBufferObject, PartialAudioBufferObject, ChunkAudioObject, VideoTranslationAudioRequest/Response

```proto
message AudioBufferObject {
  bytes audioFile = 2;   // Uint8Array аудио (вебм/opus и т.п.)
  string fileId = 1;     // JSON со сведениями о типе загрузки и размере файла
}

message PartialAudioBufferObject {
  bytes audioFile = 2;
  int32 chunkId = 1;     // индекс чанка (0..N)
}

message ChunkAudioObject {
  PartialAudioBufferObject audioBuffer = 1;
  int32 audioPartsLength = 2;  // количество чанков
  string fileId = 3;           // JSON с info о файле
  int32 version = 4;           // текущая версия форматирования (обычно 1)
}

message VideoTranslationAudioRequest {
  string translationId = 1;
  string url = 2;
  optional ChunkAudioObject partialAudioInfo = 4; // один из двух вариантов
  optional AudioBufferObject audioInfo = 6;
}

message VideoTranslationAudioResponse {
  int32 status = 1;             // 1 = ждём чанки, 2 = всё загружено
  repeated string remainingChunks = 2; // список оставшихся чанков/символов состояния
}
```

- В `VOTClient.requestVtransAudio` этот протокол реализован типами `AudioBufferObject`, `PartialAudioBufferObject` и `PartialAudioObject`.
- Библиотека позволяет как загружать аудио **одним файлом**, так и **почанково**.

### 3.6. SubtitlesRequest/Response

```proto
message SubtitlesObject {
  string language = 1;
  string url = 2;
  int32 unknown0 = 3;
  string translatedLanguage = 4;
  string translatedUrl = 5;
  int32 unknown1 = 6;
  int32 unknown2 = 7;
}

message SubtitlesRequest {
  string url = 1;
  string language = 2; // язык оригинала
}

message SubtitlesResponse {
  bool waiting = 1; // true, если субтитры ещё готовятся
  repeated SubtitlesObject subtitles = 2;
}
```

- Используется в методе `getSubtitlesYAImpl`.
- Клиент преобразует `SubtitlesObject` в упрощённый массив, содержащий пары `language/url` и `translatedLanguage/translatedUrl`.

### 3.7. Стримы: StreamTranslation*, StreamPingRequest

```proto
message StreamTranslationObject {
  string url = 1;       // HLS/stream URL перевода
  string timestamp = 2; // тайминг в мс начала сегмента
}

message StreamTranslationRequest {
  string url = 1;
  string language = 2;
  string responseLanguage = 3;
  int32 unknown0 = 5;
  int32 unknown1 = 6;
}

enum StreamInterval {
  NO_CONNECTION = 0;
  TRANSLATING = 10;
  STREAMING = 20;
}

message StreamTranslationResponse {
  StreamInterval interval = 1;
  optional StreamTranslationObject translatedInfo = 2;
  optional int32 pingId = 3;
}

message StreamPingRequest { int32 pingId = 1; }
```

- Это позволяет `VOTClient.translateStream` и `pingStream` реализовывать работу с live-потоками.

### 3.8. Сессии: YandexSessionRequest/Response

```proto
message YandexSessionRequest {
  string uuid = 1;   // случайный UUID клиента
  string module = 2; // например, "video_translation"
}

message YandexSessionResponse {
  string secretKey = 1;
  int32 expires = 2; // TTL в секундах
}
```

- Эти структуры напрямую используются в `YandexSessionProtobuf` и `MinimalClient.createSession`.

---

## 4. Итоговая сводка

1. `vot.js` предоставляет высокоуровневый TypeScript-клиент к Яндекс VOT API и собственному VOT Backend API, инкапсулируя работу с **protobuf-протоколом, криптографией, сессиями и нестандартными случаями (HLS/стримы, chunked audio)**.
2. Все запросы к Яндексу делаются через единый метод `request`, где тело — protobuf, а заголовки формируются функциями из `@vot.js/shared/secure` (HMAC-подписи, временные токены, имитация реального браузера/клиента).
3. Перевод видео, загрузка/дочитка аудио, субтитры и стримы реализованы с детализированной моделью статусов, что позволяет внешним приложениям (`voice-over-translation`, `vot-cli`, `vot-cli-live` и др.) строить надёжный UX поверх закрытого Яндекс-протокола, не занимаясь низкоуровневыми деталями сети и protobuf. 
