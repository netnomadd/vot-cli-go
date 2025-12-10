# Анализ protobuf-схем, используемых в `voice-over-translation`

> Источник схем: `FOSWLY/vot.js`, файл `packages/shared/src/protos/yandex.proto` и обёртка `packages/core/src/protobuf.ts`.
> Сам репозиторий `ilyhalight/voice-over-translation` **не содержит `.proto`**, а использует эти сообщения через библиотеку `@vot.js/ext` / `@vot.js/core`.

## 1. Общий обзор

1. Протобуф описывает бинарный протокол между клиентом (браузер/`vot.js`) и backend-частью Яндекс VOT:
   - перевод видео (однократный, не-стрим) — запрос/ответ перевода, кеш, загрузка исходного аудио;
   - субтитры — запрос списка дорожек и их переводов;
   - перевод стримов — запрос, ответ со статусом и ping для поддержания сессии;
   - сессии — получение секретного ключа по UUID/модулю.
2. В `vot.js` эти сообщения инкапсулированы в классах `YandexVOTProtobuf` и `YandexSessionProtobuf`, которые:
   - **кодируют** данные TypeScript-структур в `Uint8Array` для отправки на сервер;
   - **декодируют** бинарный ответ сервера в JS-объекты.
3. В `voice-over-translation` эти протобуфы используются опосредованно через методы клиента `VOTClient`/`VOTWorkerClient` (`translateVideo`, `translateStream`, `getSubtitles`, `requestVtransAudio`, `getTranslationCache` и т.п.).

---

## 2. Блок «Перевод видео»

### 2.1. `VideoTranslationHelpObject`

```proto
message VideoTranslationHelpObject {
  string target = 1;    // "video_file_url" или "subtitles_file_url"
  string targetUrl = 2; // прямая ссылка на mp4/webm или VTT
}
```

- Используется как элемент массива `translationHelp` в `VideoTranslationRequest`.
- В `voice-over-translation` это поле заполняется, когда у расширения есть прямой доступ к файлу видео и/или отдельному файлу субтитров (например, через `translationHelp` в `advanced-translation`).
- Позволяет серверу Яндекса работать даже с нестандартными/временными ссылками, которые не распознаются как поддерживаемые сайты.

### 2.2. `VideoTranslationRequest`

```proto
message VideoTranslationRequest {
  string url = 3;
  optional string deviceId = 4;  // в мобильной версии
  bool firstRequest = 5;         // первый запрос или последующий ретрай
  double duration = 6;           // длительность видео в секундах
  int32 unknown0 = 7;            // 1 1
  string language = 8;           // исходный язык
  bool forceSourceLang = 9;      // 0 = авто, 1 = язык выбран пользователем
  int32 unknown1 = 10;           // 0 0
  repeated VideoTranslationHelpObject translationHelp = 11;
  bool wasStream = 13;           // было ли видео стримом
  string responseLanguage = 14;  // целевой язык
  int32 unknown2 = 15;           // 1?
  int32 unknown3 = 16;           // до 04.2025 — 1, сейчас 2 (версия протокола)
  bool bypassCache = 17;         // попытка обойти кеш перевода
  bool useLivelyVoice = 18;      // включить режим Lively Voice (новые голоса)
  string videoTitle = 19;        // заголовок видео (YouTube-тайтл)
}
```

**Связь с `voice-over-translation`:**

- Формируется в `vot.js` методом `YandexVOTProtobuf.encodeTranslationRequest(...)`, который вызывается внутри `VOTClient.translateVideo`.
- Параметры берутся из `videoData` и опций перевода, которые собирает `VideoHandler` и `VOTTranslationHandler`:
  - `url` — URL страницы/видео (или URL, полученный через helpers в `@vot.js`);
  - `duration` — длительность, которую расширение получает из `<video>` или `getVideoData`;
  - `language`/`responseLanguage` — `requestLang`/`responseLang` из настроек интерфейса;
  - `forceSourceLang` — true, если пользователь явно выбрал язык (см. advanced-translation);
  - `wasStream` — true для бывших стримов YouTube (важно для корректной работы сервиса);
  - `bypassCache` — опция «обойти кеш» (используется редко, поведение нестабильно);
  - `useLivelyVoice` — включается, если пользователь выбрал Lively Voice и прошёл авторизацию;
  - `videoTitle` — заголовок (title) YouTube-видео (для моделей, зависящих от контекста).

На стороне `voice-over-translation` это сообщение соответствует пользовательскому действию «запустить перевод» (кнопка/автостарт).

### 2.3. `VideoTranslationResponse`

```proto
message VideoTranslationResponse {
  optional string url = 1;         // ссылка на готовый аудиофайл перевода
  optional double duration = 2;    // длительность результата
  int32 status = 4;                // статус перевода
  optional int32 remainingTime = 5;// оставшееся время до готовности
  optional int32 unknown0 = 6;     // 0 (первый запрос) или 10 (последующие)
  string translationId = 7;        // ID сессии перевода
  optional string language = 8;    // фактически определённый язык исходника
  optional string message = 9;     // сообщение об ошибке/состоянии
  bool isLivelyVoice = 10;         // перевод сделан Lively Voice-моделью
  optional int32 unknown2 = 11;    // 0/1 при status=success
  optional int32 shouldRetry = 12; // 1 = следует повторить (ошибка аудио)
  optional int32 unknown3 = 13;    // 1, если «классические голоса» и есть url
}
```

**Интерпретация в `voice-over-translation`:**

- `status` и `remainingTime` используются в `VOTTranslationHandler` для:
  - отображения статуса в UI (ожидание, перевод, ошибка);
  - расчёта интервалов повтора запросов `translateVideo`.
- `translationId` — критичен для **загрузки оригинального аудио**:
  - при `status` «AUDIO_REQUESTED» (логическое состояние, выведенное из ответа) `AudioDownloader` начинает качать аудио, а `VOTTranslationHandler` вызывает `requestVtransAudio` с этим `translationId`.
- `url` — конечная ссылка на переведённое аудио,
  - передаётся в `VideoHandler.updateTranslation()`, где:
    - при необходимости валидируется (`HEAD`/`range bytes=0-0`),
    - может быть пропроксирована через воркер для S3-доменов.
- `isLivelyVoice` — позволяет интерфейсу показать, что используется «живой» голос.
- `message` — отображается пользователю как текст ошибки/состояния.

### 2.4. Кеш перевода

```proto
message VideoTranslationCacheItem {
  int32 status = 1;
  optional int32 remainingTime = 2;
  optional string message = 3; // при status=3
  optional int32 unknown0 = 4; // при status=3, значение 5
}

message VideoTranslationCacheRequest {
  string url = 1;
  double duration = 2;
  string language = 3;
  string responseLanguage = 4;
}

message VideoTranslationCacheResponse {
  VideoTranslationCacheItem default = 1;
  VideoTranslationCacheItem cloning = 2;
}
```

- Кеш-запросы/ответы используются для быстрой проверки, есть ли уже результат перевода/клонированного голоса для комбинации `(url, duration, srcLang, dstLang)`.
- В `vot.js` кодируются/декодируются через `encodeTranslationCacheRequest`/`decodeTranslationCacheResponse` и прозрачно инкапсулированы в `VOTClient`.
- Для `voice-over-translation` это выглядит как мгновенный ответ о состоянии, без дополнительных HTTP-запросов к UI.

### 2.5. Загрузка исходного аудио

Этот блок критичен для новых версий VOT, где требуется отправка **байтов оригинального аудио**:

```proto
message AudioBufferObject {
  bytes audioFile = 2; // полный файл (может быть пустым при ошибке)
  string fileId = 1;   // JSON с инфо о загрузчике/формате
}

message PartialAudioBufferObject {
  bytes audioFile = 2; // часть файла
  int32 chunkId = 1;   // индекс чанка
}

message ChunkAudioObject {
  PartialAudioBufferObject audioBuffer = 1;
  int32 audioPartsLength = 2; // общее число чанков
  string fileId = 3;          // JSON как в AudioBufferObject
  int32 version = 4;          // текущая версия = 1
}

message VideoTranslationAudioRequest {
  string translationId = 1;
  string url = 2;
  optional ChunkAudioObject partialAudioInfo = 4; // при чанковой загрузке
  optional AudioBufferObject audioInfo = 6;       // при цельном файле
}

message VideoTranslationAudioResponse {
  int32 status = 1;                // 1 = ждём чанки, 2 = всё загружено
  repeated string remainingChunks = 2; // список идентификаторов/номеров
}
```

**Как этим пользуется `voice-over-translation`:**

1. При статусе «нужно загрузить аудио» (`shouldSendFailedAudio/extra статус`):
   - `AudioDownloader` извлекает URL и/или байты аудио с сайта (через iframe и стратегию `web_api_get_all_generating_urls_data_from_iframe`).
   - Формирует JSON `fileId` с полями `downloadType`, `itag`, `minChunkSize`, `fileSize`.
2. `VOTTranslationHandler` вызывает `VOTClient.requestVtransAudio(...)`, который внутри использует `YandexVOTProtobuf.encodeTranslationAudioRequest`:
   - если отправляется один файл — `audioInfo` + пустой `partialAudioInfo`;
   - если чанки — `partialAudioInfo` + `audioBuffer.chunkId`, `audioPartsLength` и общий `fileId`.
3. Ответ `VideoTranslationAudioResponse` говорит клиенту:
   - `status=1` — сервер ждёт ещё чанки, надо продолжать загрузку;
   - `status=2` — можно повторно запрашивать `translateVideo` и ожидать готовый перевод.

---

## 3. Блок «Субтитры»

```proto
message SubtitlesObject {
  string language = 1;           // язык оригинала
  string url = 2;                // ссылка на файл сабов
  int32 unknown0 = 3;
  string translatedLanguage = 4; // язык перевода
  string translatedUrl = 5;      // ссылка на переведённые сабы
  int32 unknown1 = 6;
  int32 unknown2 = 7;
}

message SubtitlesRequest {
  string url = 1;      // URL видео/страницы
  string language = 2; // исходный язык
}

message SubtitlesResponse {
  bool waiting = 1;                // 1 = перевод/генерация ещё идёт
  repeated SubtitlesObject subtitles = 2;
}
```

**Использование в `voice-over-translation`:**

- В `SubtitlesProcessor.getSubtitles` вызывается `client.getSubtitles({ videoData, requestLang })`:
  - `VOTClient` кодирует запрос в `SubtitlesRequest` и декодирует `SubtitlesResponse`.
- Поля `url`/`translatedUrl` указывают на:
  - оригинальные сабы Яндекса,
  - переведённые сабы (генерируемые моделью).
- `waiting=true` позволяет UI показать, что сабы ещё не готовы; при повторном запросе `waiting` станет `false`, и массив `subtitles` будет заполнен.
- Расширение затем по этим URL скачивает VTT/JSON, конвертирует в единый формат и рисует в `SubtitlesWidget`.

---

## 4. Блок «Перевод стримов»

```proto
message StreamTranslationObject {
  string url = 1;      // ссылка на M3U8-поток с переведённым аудио
  string timestamp = 2;// таймстамп начала потока, строкой (ms)
}

message StreamTranslationRequest {
  string url = 1;
  string language = 2;         // исходный язык
  string responseLanguage = 3; // язык перевода
  int32 unknown0 = 5;
  int32 unknown1 = 6;
}

enum StreamInterval {
  NO_CONNECTION = 0;
  TRANSLATING = 10;
  STREAMING = 20;
}

message StreamTranslationResponse {
  StreamInterval interval = 1;               // 0/10/20
  optional StreamTranslationObject translatedInfo = 2;
  optional int32 pingId = 3;                 // ID для ping-сообщений
}

message StreamPingRequest {
  int32 pingId = 1;
}

// ответ на ping не описан отдельным proto, сервер возвращает простой статус
```

**Связка с `voice-over-translation`:**

- В гайде `Translating stream` (документация) показан явный пример использования:
  - периодически вызывать `translateStream`, пока `interval=TRANSLATING (10)` и `translated=false`;
  - при `interval=STREAMING (20)` и наличии `translatedInfo.url` — начинать воспроизведение;
  - `pingId` использовать для периодических `pingStream({ pingId })`, чтобы поток не остановился.
- Внутри `vot.js`:
  - `encodeStreamRequest` и `decodeStreamResponse` реализуют кодирование/декодирование;
  - `encodeStreamPingRequest` — упаковка `pingId`.
- В `voice-over-translation` вся логика обёрнута в `translateStream`/`pingStream` клиента; поток всегда фактически воспроизводится через `media-proxy` (см. общий анализ репозитория).

---

## 5. Блок «Сессии»

```proto
message YandexSessionRequest {
  string uuid = 1;  // ID клиента/сессии
  string module = 2;// e.g. "video_translation" или "summarization"
}

message YandexSessionResponse {
  string secretKey = 1; // секретный ключ для подписи/шифрования
  int32 expires = 2;    // время жизни, сек
}
```

**Роль для `voice-over-translation`:**

- Сессионные сообщения используются на уровне `vot.js` для установления защищённого канала с backend-ом Яндекса:
  - формирование заголовков/подписей, необходимых для скрытого API браузера;
  - выбор модуля (`video_translation`, `summarization` и прочие).
- В `voice-over-translation` это не видно напрямую — расширение лишь оперирует высокоуровневыми методами клиента `VOTClient`, но именно через эти protobuf-сообщения клиент синхронизирует сессию с сервером.

---

## 6. Как всё это связано с кодом `voice-over-translation`

1. `voice-over-translation` **не сериализует protobuf самостоятельно**:
   - вся работа с бинарным протоколом инкапсулирована в `@vot.js/core` + `@vot.js/shared` (`yandex.proto` → `yandex.ts` → `YandexVOTProtobuf`).
2. Расширение лишь задаёт **высокоуровневые параметры**, которые попадают в protobuf-поля:
   - `requestLang`, `responseLang`, `videoTitle`, `wasStream`, `bypassCache`, `useLivelyVoice`, `translationHelp`, `shouldSendFailedAudio` и др.;
   - эти поля потом проходят через `YandexVOTProtobuf.encode*` в сообщения `VideoTranslationRequest`, `VideoTranslationAudioRequest`, `SubtitlesRequest`, `StreamTranslationRequest` и т.д.
3. Полученные от сервера бинарные ответы декодируются в объекты `VideoTranslationResponse`, `SubtitlesResponse`, `StreamTranslationResponse`, `VideoTranslationAudioResponse`, `YandexSessionResponse` и используются:
   - для управления очередями запросов (ожидание, ретраи, пинг стримов);
   - для загрузки/переиспользования аудио и сабов (по `url`/`translatedUrl`);
   - для отображения статусов и ошибок в UI (`message`, `remainingTime`, `waiting`, `interval`).
4. Таким образом, описанные protobuf-сообщения — это **формальный протокол** между экосистемой VOT (включая `voice-over-translation`) и закрытым Yandex VOT API, а репозиторий расширения лишь надстраивает над ним интерфейс, логику хелперов и интеграцию с браузером.