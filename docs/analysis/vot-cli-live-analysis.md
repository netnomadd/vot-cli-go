# Анализ репозитория `fantomcheg/vot-cli-live`

## 1. Общий принцип работы

1. `vot-cli-live` — форк `FOSWLY/vot-cli`, это CLI на Node.js, которое через терминал запрашивает у Яндекс-сервиса видеоперевода **переведённую аудиодорожку или субтитры**, а также (дополнительно) может **собрать видео с этим переводом**, используя `yt-dlp` и `ffmpeg`.
2. Основной бинарь — `src/index.js` (задан как `bin` и `main` в `package.json`), который:
   - парсит аргументы (`minimist`),
   - валидирует/нормализует ссылки на видео (`utils/validator.js`, `config/sites.js`, `utils/getVideoId.js`),
   - при необходимости разрешает спец-кейсы (CourseHunter — `utils/coursehunter.js`),
   - получает реальную длительность и (опционально) название видео через `yt-dlp` (`utils/getVideoDuration.js`, `utils/getVideoTitle.js`),
   - вызывает Яндекс API через `yandexRequests.js` / `yandexRawRequest.js` с protobuf-сообщениями, собранными в `yandexProtobuf.js`,
   - интерпретирует ответы, скачивает аудио/сабы (`download.js`), а при опции `--merge-video` — скачивает оригинальное видео и склеивает его с переводом (`mergeVideo.js`).
3. Утилита поддерживает два типа озвучки: стандартный TTS и **живые голоса** (параметр `--voice-style=tts|live`, логика — флаг `useLivelyVoice/useLiveVoices`, поле `useLivelyVoice` в protobuf-запросе).

---

## 2. Куда, как и зачем отправляет запросы

### 2.1. Яндекс-видеоперевод (`api.browser.yandex.ru`)

#### Конфигурация

Файл: `src/config/config.js`

```js
const debug = false;
const workerHost = "api.browser.yandex.ru";

const yandexHmacKey = "bt8xH3VOlb4mqf0nqAibnDOoiPlXsisf";
const yandexUserAgent =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 YaBrowser/24.4.0.0 Safari/537.36";

export { debug, workerHost, yandexHmacKey, yandexUserAgent };
```

- Все вызовы Яндекс API идут на домен `api.browser.yandex.ru` — это тот же закрытый API, который использует Яндекс.Браузер для видеоперевода.
- `yandexHmacKey` и `yandexUserAgent` используются для маскировки под официальный клиент: HMAC-подпись и заголовок `User-Agent` совпадают с браузером.

#### Низкоуровневый HTTP-клиент

Файл: `src/yandexRawRequest.js`

```js
import axios from "axios";
import { workerHost, yandexUserAgent } from "./config/config.js";
...
export default async function yandexRawRequest(
  path,
  body,
  headers,
  proxyData,
  callback,
  timeout = 60000,
) {
  await axios({
    url: `https://${workerHost}${path}`,
    method: "post",
    timeout,
    headers: {
      Accept: "application/x-protobuf",
      "Accept-Language": "en",
      "Content-Type": "application/x-protobuf",
      "User-Agent": yandexUserAgent,
      Pragma: "no-cache",
      "Cache-Control": "no-cache",
      "Sec-Fetch-Mode": "no-cors",
      "sec-ch-ua": null,
      "sec-ch-ua-mobile": null,
      "sec-ch-ua-platform": null,
      ...headers,
    },
    proxy: proxyData,
    responseType: "arraybuffer",
    data: body,
  })
    .then((response) => {
      callback(response.status === 200, response.data);
    })
    .catch((err) => {
      const status = err.response?.status;
      const statusText = err.response?.statusText;
      const errorCode = err.code;
      const baseMessage = err.message;

      let message;
      if (errorCode === "ECONNABORTED") {
        message = `Yandex API request timeout (${timeout}ms exceeded)`;
      } else if (errorCode === "ECONNRESET") {
        message = `Yandex API connection reset. Try using a proxy with --proxy`;
      } else if (status !== undefined) {
        message = `Yandex API request failed with status ${status}${
          statusText ? ` ${statusText}` : ""
        }`;
      } else if (errorCode) {
        message = `Yandex API request failed: ${errorCode}`;
      } else {
        message = `Yandex API request failed: ${baseMessage}`;
      }

      callback(false, message);
    });
}
```

**Куда:**

- `https://api.browser.yandex.ru/video-translation/translate` — запрос перевода видео, получение URL переведённой аудиодорожки.
- `https://api.browser.yandex.ru/video-subtitles/get-subtitles` — запрос списка субтитров (исходных и переведённых).

**Как:**

- Тело запроса — бинарный protobuf (см. раздел 3, `yandexProtobuf.js`).
- Заголовки имитируют Яндекс.Браузер.
- `proxyData` (подготовлен в `src/proxy.js` из `--proxy`) передаётся прямо в `axios` как объект с `protocol`, `host`, `port`, `auth` и, для некоторых операций, `proxyUrl` (строка для `yt-dlp`/`ffmpeg`).
- Ошибки оборачиваются в человекочитаемые сообщения, чтобы CLI мог корректно их показать.

**Зачем:**

- Это ключевой канал, через который CLI получает доступ к сервису видеоперевода Яндекса без промежуточных серверов.

#### Высокоуровневые обёртки

Файл: `src/yandexRequests.js`

```js
async function requestVideoTranslation(
  url,
  duration,
  requestLang,
  responseLang,
  translationHelp,
  proxyData,
  callback,
  useLiveVoices = true,
) {
  const body = yandexProtobuf.encodeTranslationRequest(
    url,
    duration,
    requestLang,
    responseLang,
    translationHelp,
    useLiveVoices,
  );
  await yandexRawRequest(
    "/video-translation/translate",
    body,
    {
      "Vtrans-Signature": await getSignature(body),
      "Sec-Vtrans-Token": getUUID(false),
    },
    proxyData,
    callback,
  );
}

async function requestVideoSubtitles(url, requestLang, proxyData, callback) {
  const body = yandexProtobuf.encodeSubtitlesRequest(url, requestLang);
  await yandexRawRequest(
    "/video-subtitles/get-subtitles",
    body,
    {
      "Vsubs-Signature": await getSignature(body),
      "Sec-Vsubs-Token": getUUID(false),
    },
    proxyData,
    callback,
  );
}
```

- `getSignature(body)` (`utils/getSignature.js`) вычисляет HMAC-SHA256 подпись тела на ключе `yandexHmacKey` и возвращает строку в hex-формате, которую Яндекс ждёт в заголовках.
- `getUUID(false)` (`utils/getUUID.js`) генерирует 32-символьный hex-токен (аналог `Sec-Vtrans-Token`/`Sec-Vsubs-Token`).

**Зачем:**

- Скрыть детали формирования protobuf и криптографии, предоставив простые вызовы «запросить перевод видео» и «запросить субтитры».

### 2.2. Скачивание перевода и субтитров (любой домен из ответа Яндекса)

Файл: `src/download.js`

```js
export default async function downloadFile(url, outputPath, subtask, videoId) {
  if (!url) throw new Error("Invalid download link");
  const IS_NEED_CONVERT = outputPath.endsWith(".srt");
  const writer = fs.createWriteStream(outputPath);
  const { data, headers } = await axios({
    method: "get",
    url,
    responseType: "stream",
  });

  const totalLength = headers["content-length"];
  let downloadedLength = 0;

  data.on("data", (chunk) => {
    downloadedLength += chunk.length;
    if (subtask) {
      subtask.title = `Downloading ${videoId}: ${((downloadedLength / totalLength) * 100).toFixed(1)}%`;
    }
  });

  if (IS_NEED_CONVERT) {
    let dataBuffer = "";
    const writableStream = new Writable({
      write(chunk, encoding, callback) {
        dataBuffer += chunk.toString();
        callback();
      },
    });
    data.pipe(writableStream);
    data.on("end", () => {
      const jsonData = JSON.parse(dataBuffer);
      writer.write(jsonToSrt(jsonData["subtitles"]));
      writer.end();
    });
  } else {
    data.pipe(writer);
  }

  return new Promise((resolve, reject) => {
    writer.on("finish", resolve);
    writer.on("error", reject);
  });
}
```

**Куда:**

- В `url` передаётся значение из protobuf-ответа Яндекса:
  - для аудио перевода — обычно ссылка на S3 Яндекса типа `https://vtrans.s3-private.mds.yandex.net/...` (или ближайший CDN/прокси),
  - для сабов — ссылка на JSON с полем `subtitles`.
- Сам домен не хардкожен в коде — CLI доверяет URL, который вернул Яндекс.

**Зачем:**

- Физически скачать результат перевода или субтитры и сохранить их на диск пользователя (с прогрессом и, при необходимости, конвертацией JSON → `.srt`).

### 2.3. CourseHunter (`coursehunter.net`)

Файл: `src/utils/coursehunter.js`

- Делает запросы:
  - `GET https://coursehunter.net/course/{videoId}` — HTML-страница курса, парсится через `jsdom` для извлечения `course_id`;
  - `GET https://coursehunter.net/api/v1/course/{courseId}/lessons` — JSON со списком уроков, из него берётся поле `file` (прямой URL видео).
- Эта логика используется только если `validator` и `getVideoId` распознали ссылку как CourseHunter.

**Зачем:**

- Превратить человеко-читаемую ссылку на урок (или курс) CourseHunter в прямой URL на видео, чтобы затем передать его в Яндекс API для перевода.

### 2.4. yt-dlp и ffmpeg (через `child_process.exec`)

#### Получение длительности и названия

Файлы:
- `src/utils/getVideoDuration.js`
- `src/utils/getVideoTitle.js`

**Запросы:**

- Локальные команды (через `exec`):
  - `yt-dlp --version` — проверка наличия yt-dlp;
  - `yt-dlp --print duration "<url>"` — получение длительности видео в секундах;
  - `yt-dlp --get-title "<url>"` — получение названия видео.
- Возможна передача `--proxy` и переменных окружения `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY` из `proxyData.proxyUrl`, чтобы yt-dlp ходил через тот же прокси.

**Зачем:**

- Реальная длительность (`duration`) подставляется в protobuf-запрос к Яндексу вместо фиксированного 341 c (улучшает соответствие ожиданиям API и временные лимиты).
- Название видео используется для красивого имени файла (`<title>.mp3` / `<title>.mp4`), если не указано `--output-file`.

#### Скачивание и склейка видео

Файл: `src/mergeVideo.js`

- Для **скачивания видео**:

  ```js
  yt-dlp -f "best[ext=mp4]/best" --merge-output-format mp4 -o "<temp_video_...>.mp4" [--proxy <proxyUrl>] "<videoUrl>"
  ```

  - Использует yt-dlp для скачивания лучшего доступного MP4.
  - Есть таймаут 10 минут на загрузку; при превышении выбрасывается отдельная ошибка.

- Для **склейки с переводом** (`ffmpeg`):

  - Если `keepOriginalAudio = true` (по умолчанию):

    ```bash
    ffmpeg -i "video.mp4" -i "audio.mp3" \
      -filter_complex "[0:a]volume=<originalVolume>[a1];[1:a]volume=<translationVolume>[a2];[a1][a2]amix=inputs=2:duration=longest[aout]" \
      -map 0:v -map "[aout]" -c:v copy -c:a aac -b:a 192k -y "output.mp4"
    ```

  - Если `keepOriginalAudio = false` — заменяет оригинальный звук переводом:

    ```bash
    ffmpeg -i "video.mp4" -i "audio.mp3" -map 0:v -map 1:a -c:v copy -c:a aac -b:a 192k -shortest -y "output.mp4"
    ```

- Команды ограничены по времени (15 минут по умолчанию) через `execWithTimeout`.

**Зачем:**

- Реализовать фичу `--merge-video`: выдавать пользователю **готовые видеофайлы с встроенным переводом**, в том числе с миксом оригинала и перевода с регулируемой громкостью.

### 2.5. Прокси и сетевые настройки

Файл: `src/proxy.js`

- Парсит строку вида `[<PROTOCOL>://]<USERNAME>:<PASSWORD>@<HOST>[:<port>]` в объект:

  ```js
  {
    protocol: "http" | "https",
    host: "...",
    port: "..." | "",
    auth?: { username, password },
    proxyUrl: "protocol://username:password@host:port" // (расширение в этом форке)
  }
  ```

- Этот объект используется:
  - как `proxy` в `axios` (Яндекс API),
  - как `proxyUrl`/env для `yt-dlp` и `ffmpeg`.

**Зачем:**

- Позволить пользователю вручную настроить маршрутизацию всего сетевого трафика (API + загрузка исходного видео) через SOCKS/HTTP(S)-прокси, чтобы обходить блокировки или ограничивать утечки IP.

---

## 3. Структура protobuf-сообщений Яндекс API

Файл: `src/yandexProtobuf.js`

Весь протокол описан через `protobufjs` программно, без `.proto` файлов. Ниже — схема с комментариями.

### 3.1. VideoTranslationHelpObject

```proto
message VideoTranslationHelpObject {
  string target    = 1; // "video_file_url" или "subtitles_file_url"
  string targetUrl = 2; // URL файла видео или сабов
}
```

- Используется для передачи дополнительной информации Яндексу (например, прямых ссылок на файл/сабы для Coursera/Udemy).

### 3.2. VideoTranslationRequest

```proto
message VideoTranslationRequest {
  // 3: исходный URL видео (YouTube, VK, прямой mp4 и т.п.)
  string url             = 3;

  // 4: deviceId (используется мобильной версией, здесь не заполняется)
  string deviceId        = 4;

  // 5: признак первого запроса (true при первом обращении, false при последующих опросах)
  bool   firstRequest    = 5;

  // 6: длительность видео в секундах (double)
  double duration        = 6;

  // 7: unknown0 — в коде всегда = 1
  int32  unknown0        = 7;

  // 8: language — язык оригинала (коды из availableLangs, например "en", "ru", "auto" и т.п.)
  string language        = 8;

  // 9: forceSourceLang — если true, принудительно использовать language, не автоопределять
  bool   forceSourceLang = 9;

  // 10: unknown1 — в коде всегда = 0
  int32  unknown1        = 10;

  // 11: translationHelp — массив вспомогательных описаний (VideoTranslationHelpObject)
  repeated VideoTranslationHelpObject translationHelp = 11;

  // 13: wasStream — признак того, что это закончившийся стрим (здесь всегда false)
  bool   wasStream       = 13;

  // 14: responseLanguage — целевой язык перевода (например "ru", "en")
  string responseLanguage = 14;

  // 15: unknown2 — в коде всегда = 1
  int32  unknown2        = 15;

  // 16: unknown3 — до апреля 2025 был 1, сейчас 2 (комментарий в коде)
  int32  unknown3        = 16;

  // 17: bypassCache — принудительно игнорировать кеш перевода (false по умолчанию)
  bool   bypassCache     = 17;

  // 18: useLivelyVoice — КЛЮЧЕВОЕ поле этого форка: использовать ли живые голоса
  bool   useLivelyVoice  = 18;

  // 19: videoTitle — название видео (сейчас заполняется пустой строкой)
  string videoTitle      = 19;
}
```

В этом форке `encodeTranslationRequest` заполняет поля так:

```js
encodeTranslationRequest(url, duration, requestLang, responseLang, translationHelp, useLiveVoices=true) {
  return root.VideoTranslationRequest.encode({
    url,
    firstRequest: true,
    duration,
    unknown0: 1,
    language: requestLang,
    forceSourceLang: false,
    unknown1: 0,
    translationHelp,
    wasStream: false,
    responseLanguage: responseLang,
    unknown2: 1,
    unknown3: 2,
    bypassCache: false,
    useLivelyVoice: useLiveVoices,
    videoTitle: "",
  }).finish();
}
```

### 3.3. VideoTranslationResponse

```proto
message VideoTranslationResponse {
  string url          = 1; // URL аудиофайла перевода (может быть null на промежуточных стадиях)
  double duration     = 2; // длительность перевода (сек)
  int32  status       = 4; // 0=ошибка, 1=готово, 2=перевод в процессе
  int32  remainingTime = 5; // оставшееся время (сек) до готовности перевода (подсказка для опроса)
  int32  unknown0     = 6; // 0 (первый запрос) -> 10 (повторные)
  string unknown1     = 7;
  string language     = 8; // детектированный язык оригинала
  string message      = 9; // текст ошибки или статуса для пользователя
}
```

CLI интерпретирует это так (в `translateVideo.js`):

- `status == 0` → ошибка, берётся `message` и возвращается пользователю.
- `status == 1` → успех, берётся `url`; если `url == null`, ошибка `"Audio link not received"`.
- `status == 2` → «перевод займёт несколько минут», CLI запускает цикл повторных запросов (с интервалом 30s до лимита по количеству).

### 3.4. VideoSubtitlesRequest / Response

```proto
message VideoSubtitlesRequest {
  string url      = 1; // исходный URL видео
  string language = 2; // язык оригинала
}

message VideoSubtitlesObject {
  string language           = 1; // язык субтитров (оригинал)
  string url                = 2; // URL JSON с сабами
  int32  unknown2           = 3;
  string translatedLanguage = 4; // язык перевода сабов (если есть)
  string translatedUrl      = 5; // URL JSON с переведёнными сабами
  int32  unknown5           = 6;
  int32  unknown6           = 7;
}

message VideoSubtitlesResponse {
  int32                      unknown0  = 1;
  repeated VideoSubtitlesObject subtitles = 2;
}
```

- CLI сводит это к массиву объектов вида:

  ```js
  {
    source: "yandex",
    language: yaSubtitlesObject.language,
    url: yaSubtitlesObject.url,
    // либо translatedLanguage + translatedUrl
  }
  ```

- При скачивании выбирается элемент, где `language === RESPONSE_LANG`.

### 3.5. VideoStreamRequest / Response / Ping (на будущее)

```proto
message VideoStreamRequest {
  string url             = 1;
  string language        = 2;
  string responseLanguage = 3;
}

message VideoStreamPingRequest {
  int32 pingId = 1;
}

message VideoStreamObject {
  string url     = 1;
  int64  timestamp = 2; // время (мс) запроса/стартового кадра
}

message VideoStreamResponse {
  int32             interval      = 1; // 20=стрим идёт, 10=идёт перевод, 0=нет соединения
  VideoStreamObject translatedInfo = 2; // информация о переведённом отрезке
  int32             pingId        = 3;
}
```

- В этом проекте пока **не используется** (нет прямой логики стримов), но определения оставлены из базового проекта для совместимости.

---

## 4. Принцип работы CLI-пайплайна

### 4.1. Общий сценарий (аудио-перевод)

1. **Старт и парсинг аргументов** (`src/index.js`):
   - Читается версия из `package.json`.
   - С помощью `minimist` извлекаются:
     - `--output`, `--output-file`, `--lang`, `--reslang`, `--voice-style`, `--proxy`, `--merge-video`, `--keep-original-audio`, `--translation-volume`, `--original-volume`, флаги `--subs`/`--subs-srt` и т.п.
   - Настраиваются глобальные переменные `REQUEST_LANG`, `RESPONSE_LANG`, `USE_LIVE_VOICES` (по умолчанию `true`), `proxyData`.
2. **Валидация ссылок**:
   - Для каждой ссылки из `ARG_LINKS` вызываются `validate(url)` и `getVideoId(service.host, url)`, которые возвращают:
     - базовый `service.url`, на который нужно повесить `videoId`,
     - либо специальные структуры для некоторых хостов (CourseHunter).
   - Для CourseHunter — дополнительно `coursehunterUtils.getVideoData(...)` для получения прямого `videoUrl`.
3. **Формирование итогового URL**:
   - Для каждого видео создаётся Listr-подзадача, которая формирует `finalURL`:

     ```js
     finalURL = videoId.startsWith("https://") || service.host === "custom"
       ? videoId
       : `${service.url}${videoId}`;
     ```

   - Параллельно (если возможно) получаются:
     - название видео через `getVideoTitle(finalURL)` (yt-dlp),
     - длительность через `getVideoDuration(finalURL, proxyData)`.
4. **Запрос перевода или субтитров**:
   - Для аудио: `translate(finalURL, subtask)` → `translateVideo` → `requestVideoTranslation` → `yandexRawRequest`.
   - Для сабов: `fetchSubtitles(finalURL, subtask)` → `requestVideoSubtitles`.
   - Если Яндекс отвечает `status == 2` (перевод в процессе), CLI:
     - показывает промежуточное сообщение,
     - запускает цикл повторных запросов (до 10 попыток, каждые 30 секунд),
     - при таймауте даёт осмысленную ошибку.
5. **Скачивание результата**:
   - Для аудио: 
     - имя файла: либо `--output-file`, либо `<videoTitle>.mp3`, если название удалось получить, либо `<очищенный videoId>---<uuid>.mp3`.
     - `downloadFile(translateResult.urlOrError, OUTPUT_DIR/filename, ...)` скачивает поток и сохраняет его на диск.
   - Для сабов:
     - из массива `subsOrError` выбирается элемент с `language === RESPONSE_LANG`;
     - файл сохраняется как `.json` или `.srt` (если включён `--subs-srt`);
     - при `.srt` JSON автоматически конвертируется.
6. **Опциональная сборка видео с переводом** (`--merge-video`):
   - Сначала скачивается аудио перевода в временный файл (`downloadFile`).
   - Затем `createVideoWithTranslation(finalURL, audioPath, videoPath, { keepOriginalAudio, audioVolume, translationVolume, proxyUrl })`:
     - скачивает оригинал видео через yt-dlp (через прокси, если указан),
     - склеивает его с переводом через ffmpeg (либо микширует, либо заменяет аудио),
     - удаляет временные файлы.

---

## 5. Сводка по сетевому поведению

1. **Основной внешний сервис — Яндекс-видеоперевод** на `api.browser.yandex.ru`:
   - CLI формирует protobuf-запросы c HMAC-подписью и маскирует себя под Яндекс.Браузер.
   - Через этот API получает URL аудио перевода и URL JSON-файлов с субтитрами.
2. **Скачивание аудио/сабов** происходит напрямую по URL из ответов Яндекса (обычно `*.mds.yandex.net` или связанные CDN/прокси) с помощью `axios`.
3. **Для CourseHunter** дополнительно используются публичные HTTP-эндпоинты `coursehunter.net` (HTML + JSON API) для нахождения реального `videoUrl`.
4. **Локальные утилиты yt-dlp и ffmpeg** используются для:
   - определения длительности и названия видео (через собственные HTTP-запросы к видеохостингу);
   - скачивания исходных видеороликов и создания финальных видео с переводом (опционально через тот же прокси, что и Яндекс API).
5. **Прокси настраивается пользователем вручную** через `--proxy`, CLI не использует собственные серверы-посредники: все запросы либо идут напрямую с машины пользователя, либо через указанный им прокси.

В итоге `vot-cli-live` — это толстый клиент, который полностью управляет цепочкой: «URL видео → запрос в Яндекс API по protobuf → URL аудиоперевода/сабов → скачивание/конвертация → (опционально) сборка видео с переводом», добавляя поверх оригинального `vot-cli` поддержку живых голосов Яндекса и улучшенный UX. 
