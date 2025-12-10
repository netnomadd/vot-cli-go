# Анализ репозитория `FOSWLY/vot-cli`

## 1. Общий принцип работы

1. `vot-cli` — это Node.js CLI-утилита, которая позволяет через терминал запросить у Яндекс-сервиса видеоперевода аудио-дорожку или субтитры к видео и, при желании, сохранить их в файл.
2. Основа CLI — скрипт `src/index.js`, объявленный как `bin` в `package.json`; он:
   - разбирает аргументы командной строки (`minimist`),
   - валидирует и нормализует входные ссылки на видео (`validator.js`, `getVideoId.js`, `sites.js`),
   - при необходимости преобразует некоторые нестандартные ссылки (например, `coursehunter.net`) в прямые URL на медиа (`coursehunter.js`),
   - вызывает обёртки над Яндекс API (`yandexRequests.js` → `yandexRawRequest.js`) для получения перевода или субтитров,
   - по результатам запускает `download.js` для скачивания аудио/субтитров по выданному URL.
3. Все обращения к внешним HTTP-сервисам происходят через библиотеку `axios` (для Яндекса, для скачивания аудио/субтитров, для CourseHunter) и встроенный `fetch` (только в `coursehunter.js`), а прогресс и статус операций выводятся через `listr2` и `chalk`.

---

## 2. Ключевые модули и сети

### 2.1. Конфигурация Яндекс API (`src/config/config.js`)

```js
const debug = false;
const workerHost = "api.browser.yandex.ru";

const yandexHmacKey = "bt8xH3VOlb4mqf0nqAibnDOoiPlXsisf";
const yandexUserAgent =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 YaBrowser/24.4.0.0 Safari/537.36";

export { debug, workerHost, yandexHmacKey, yandexUserAgent };
```

- `workerHost = "api.browser.yandex.ru"` — основной целевой хост, через который CLI делает все запросы к Яндекс-видеопереводу.
- `yandexUserAgent` — строка UA браузера Яндекс, под которой CLI маскируется при обращении к API.
- `yandexHmacKey` — ключ, используемый в `getSignature.js` для вычисления HMAC подписи тел запросов (`Vtrans-Signature` / `Vsubs-Signature`), имитируя логику официального клиента Яндекс.Браузера.

### 2.2. Низкоуровневый HTTP-клиент к Яндексу (`src/yandexRawRequest.js`)

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
) {
  await axios({
    url: `https://${workerHost}${path}`,
    method: "post",
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
      console.error(err);
      callback(true, err.data);
    });
}
```

**Куда:**

- Все запросы к Яндексу идут на:
  - `https://api.browser.yandex.ru/video-translation/translate` — запрос перевода аудио-видео;
  - `https://api.browser.yandex.ru/video-subtitles/get-subtitles` — запрос субтитров.

**Как:**

- Тело (`body`) — protobuf-сообщение, закодированное в `yandexProtobuf.js`.
- Заголовки содержат:
  - `Accept`/`Content-Type: application/x-protobuf`;
  - `User-Agent: <строка Я.Браузера>`;
  - динамические подписи:
    - `Vtrans-Signature` + `Sec-Vtrans-Token` для перевода видео,
    - `Vsubs-Signature` + `Sec-Vsubs-Token` для субтитров.
- Параметр `proxy` заполняется объектом, полученным из CLI-аргумента `--proxy` (см. `src/proxy.js`), позволяя прокидывать HTTP(S)-прокси руками.

**Зачем:**

- Имитация официального клиента Яндекс.Браузера и обход возможных ограничений/валидаций по UA/подписи.
- Унифицированный низкоуровневый доступ к двум основным эндпоинтам сервиса видеоперевода.

### 2.3. Высокоуровневые запросы к Яндексу (`src/yandexRequests.js`)

```js
async function requestVideoTranslation(
  url,
  duration,
  requestLang,
  responseLang,
  translationHelp,
  proxyData,
  callback,
) {
  const body = yandexProtobuf.encodeTranslationRequest(
    url,
    duration,
    requestLang,
    responseLang,
    translationHelp,
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

- `encodeTranslationRequest` и `encodeSubtitlesRequest` формируют бинарные protobuf-запросы согласно схеме Яндекс API (описана в `yandexProtobuf.js`).
- `getSignature(body)` — вычисляет HMAC-подпись тела с использованием `yandexHmacKey`; `getUUID(false)` генерирует идентификатор токена.
- Функции принимают `proxyData`, что позволяет запускать `vot-cli` за прокси.

**Зачем:**

- Скрыть от остального кода детали работы с protobuf и криптографией.
- Дать простые вызовы вида `requestVideoTranslation(url, ...)` для бизнес-логики CLI.

### 2.4. Логика перевода и интерпретация ответов (`src/translateVideo.js`)

```js
export default async function translateVideo(
  url,
  requestLang,
  responseLang,
  translationHelp,
  proxyData,
  callback,
) {
  const duration = 341; // фиксированная длительность
  await yandexRequests.requestVideoTranslation(
    url,
    duration,
    requestLang,
    responseLang,
    translationHelp,
    proxyData,
    (success, response) => {
      if (!success) {
        callback(false, "Failed to request video translation");
        return;
      }

      const translateResponse =
        yandexProtobuf.decodeTranslationResponse(response);
      switch (translateResponse.status) {
        case 0:
          callback(false, translateResponse.message);
          return;
        case 1: {
          const hasUrl = translateResponse.url != null;
          callback(
            hasUrl,
            hasUrl ? translateResponse.url : "Audio link not received",
          );
          return;
        }
        case 2:
          callback(false, "The translation will take a few minutes");
          return;
      }
    },
  );
}
```

- CLI **не ждёт окончания перевода на стороне Яндекса**; вместо реальной длительности передаётся константа `341` секунд.
- Ответ Яндекса декодируется из protobuf; важные поля:
  - `status`:
    - `0` — ошибка (сообщение в `message`),
    - `1` — перевод готов; поле `url` содержит ссылку на аудио-файл,
    - `2` — перевод ещё готовится; пользователю выводится сообщение, а CLI позже может повторить запрос.
- Эта функция вызывается из `src/index.js` в цикле с периодическим опросом (каждые 30 секунд) при статусе «перевод готовится».

**Зачем:**

- Инкапсулировать бизнес-логику интерпретации ответа Яндекс API и выдать наружу только: `успех/ошибка` + `url` (или текст ошибки).

### 2.5. Скачивание аудио и субтитров (`src/download.js`)

```js
import fs from "fs";
import { Writable } from "stream";
import axios from "axios";
import { jsonToSrt } from "./utils/utils.js";

export default async function downloadFile(url, outputPath, subtask, videoId) {
  const IS_NEED_CONVERT = outputPath.endsWith(".srt");
  const writer = fs.createWriteStream(outputPath);
  const { data, headers } = await axios({
    method: "get",
    url: url,
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

- `url` — это либо:
  - ссылка на аудио-файл перевода от Яндекса (как правило, домены `vtrans.s3-private.mds.yandex.net` или через промежуточный прокси, если его подставил сам Яндекс),
  - либо URL субтитров (JSON) из ответа `decodeSubtitlesResponse`.

**Как и зачем:**

- Используется `axios` с `responseType: "stream"` для потокового скачивания.
- Если выходной файл заканчивается на `.srt`, CLI предполагает, что по URL лежит JSON с полем `subtitles` и конвертирует его в формат SRT с помощью `jsonToSrt()`.
- В остальном — просто прокачивает поток в `fs.WriteStream`, показывая прогресс пользователю через `listr2`.

### 2.6. Обработка субтитров Яндекса (`fetchSubtitles` в `src/index.js`)

```js
const fetchSubtitles = async (finalURL, task) => {
  let subtitlesData;

  try {
    await yandexRequests.requestVideoSubtitles(
      finalURL,
      REQUEST_LANG,
      proxyData,
      (success, response) => {
        if (!success) {
          throw new Error(chalk.red("Failed to get Yandex subtitles"));
        }

        const subtitlesResponse =
          yandexProtobuf.decodeSubtitlesResponse(response);

        let subtitles = subtitlesResponse.subtitles ?? [];
        subtitles = subtitles.reduce((result, yaSubtitlesObject) => {
          if (
            yaSubtitlesObject.language &&
            !result.find((e) => {
              if (
                e.source === "yandex" &&
                e.language === yaSubtitlesObject.language &&
                !e.translatedFromLanguage
              ) {
                return e;
              }
            })
          ) {
            result.push({
              source: "yandex",
              language: yaSubtitlesObject.language,
              url: yaSubtitlesObject.url,
            });
          }
          if (yaSubtitlesObject.translatedLanguage) {
            result.push({
              source: "yandex",
              language: yaSubtitlesObject.translatedLanguage,
              translatedFromLanguage: yaSubtitlesObject.language,
              url: yaSubtitlesObject.translatedUrl,
            });
          }
          return result;
        }, []);

        task.title = "Subtitles for the video have been received.";
        console.info(
          `Subtitles response (${finalURL}): "${chalk.gray(
            JSON.stringify(subtitles, null, 2),
          )}"`,
        );

        subtitlesData = {
          success: true,
          subsOrError: subtitles,
        };
      },
    );
  } catch (e) {
    return {
      success: false,
      subsOrError: e.message,
    };
  }

  return subtitlesData;
};
```

- Яндекс возвращает protobuf с массивом `subtitles`, каждый из которых может содержать:
  - `language`, `url` — исходные сабы;
  - `translatedLanguage`, `translatedUrl` — переведённые сабы.
- CLI сводит это к плоскому массиву объектов `{ source: "yandex", language, url, translatedFromLanguage? }` и отдаёт дальше в пайплайн скачивания.

**Зачем:**

- Позволить пользователю выбирать язык выходных субтитров (`--reslang`) и сохранять либо исходные, либо переведённые сабы.

### 2.7. Поддержка CourseHunter (`src/utils/coursehunter.js`)

```js
import { JSDOM } from "jsdom";

async function getCourseData(courseId) {
  return await fetch(
    `https://coursehunter.net/api/v1/course/${courseId}/lessons`,
  )
    .then((res) => res.json())
    .catch((err) => { ... });
}

async function parseCourseById(videoId) {
  const result = await fetch(`https://coursehunter.net/course/${videoId}`)
    .then((res) => res.text())
    .catch((err) => { ... });

  const dom = new JSDOM(result);
  const doc = dom.window.document;

  return doc.querySelector('input[name="course_id"]')?.value;
}

async function getVideoData(videoId, lessonId = 1) {
  const courseId = await parseCourseById(videoId);
  const courseData = await getCourseData(courseId);
  const lessonData = courseData?.[lessonId - 1];
  const { file: videoUrl } = lessonData;
  return { url: videoUrl };
}
```

**Куда:**

- HTTP-запросы отправляются на домен `coursehunter.net`:
  - `https://coursehunter.net/course/{videoId}` — HTML-страница курса;
  - `https://coursehunter.net/api/v1/course/{courseId}/lessons` — JSON-API c данными уроков.

**Как и зачем:**

- Используется глобальный `fetch` Node 18+ и `jsdom` для парсинга HTML.
- Эта логика задействуется в `src/index.js`, когда `validate(url)` определяет, что ссылка ведёт на CourseHunter; тогда `getVideoId` возвращает пару `[statusOrID, lessonId]`, а `coursehunterUtils.getVideoData` получает реальный `videoUrl`.
- Далее этот URL подставляется как `finalURL` для запроса к Яндексу.

### 2.8. Определение хостов и типов ссылок (`src/config/sites.js`, `validator.js`, `getVideoId.js`)

- `src/config/sites.js` содержит список поддерживаемых площадок и их базовые URL/паттерны (YouTube, Invidious/Piped-прокси, ProxiTok, PeerTube, VK и др.).
- `validator.js` проверяет, что входной URL относится к одному из поддерживаемых сервисов и возвращает объект `service` с полями `host`, `url` и т.п.
- `getVideoId.js` выдирает идентификатор видео (или пару `[courseId, lessonId]` для CourseHunter) из конкретной ссылки.

**Зачем:**

- Унифицировать дальнейшую работу: в `src/index.js` из `service` и `videoId` строится `finalURL`, который затем используется и для Яндекс-запроса, и для логгирования/именования файлов.

---

## 3. Полный путь данных при стандартном запуске

Рассмотрим типичный сценарий: `vot-cli --output=. --reslang=en "https://www.youtube.com/watch?v=..."`.

1. **Разбор аргументов:**
   - `minimist` выделяет `ARG_LINKS`, `OUTPUT_DIR`, `OUTPUT_FILE`, `REQUEST_LANG` (`--lang`), `RESPONSE_LANG` (`--reslang`), режим субтитров (`--subs`, `--subs-srt`) и прокси (`--proxy`, `--force-proxy`).
   - Параметры языка валидируются по `availableLangs` и `additionalTTS`.
   - При `--proxy=...` строка парсится `parseProxy(proxyString)` → объект `{ protocol, host, port, auth? }` для axios.
   - Если включён `--force-proxy` и прокси не удалось распарсить — выполнение прерывается.
2. **Валидация и нормализация ссылки:**
   - `validate(url)` определяет тип сервиса (YouTube, VK, CourseHunter, Invidious, Piped, ProxiTok, PeerTube и т.д.).
   - `getVideoId(service.host, url)` возвращает `videoId` (строку или `[id, lessonId]` для CourseHunter).
   - Для CourseHunter вызывается `coursehunterUtils.getVideoData(statusOrID, lessonId)` → `videoId = videoUrl`.
   - Внутри Listr-задачи формируется `finalURL`:
     - если `videoId` уже полный URL или сервис `custom`, он используется напрямую;
     - иначе подставляется `service.url + videoId`.
3. **Запрос к Яндексу:**
   - Для аудио-перевода:
     - `translate(parent.finalURL, subtask)` → `translateVideo(...)` → `requestVideoTranslation(...)` → `yandexRawRequest(...)`.
     - Тело запроса: protobuf с `url = finalURL`, `duration = 341`, `requestLang`, `responseLang` и, опционально, `translationHelp`.
     - Адрес: `https://api.browser.yandex.ru/video-translation/translate`.
     - Заголовки: protobuf, UA Я.Браузера, HMAC-подпись и UUID-токен.
     - Ответ: protobuf, который декодируется в структуру с полем `status` и, при успехе, `url` — ссылкой на аудио.
     - Если `status === 2`, CLI показывает, что перевод задерживается, и через 30 секунд повторяет запрос до получения `status === 1` или ошибки.
   - Для субтитров:
     - `fetchSubtitles(parent.finalURL, subtask)` → `requestVideoSubtitles(...)`.
     - Адрес: `https://api.browser.yandex.ru/video-subtitles/get-subtitles`.
     - Тело: protobuf с `url = finalURL` и `requestLang` (язык оригинала).
     - Ответ: protobuf с массивом вариантов сабов; CLI выбирает нужный язык по `--reslang`.
4. **Скачивание результата:**
   - Для аудио:
     - CLI выбирает имя файла: либо из `--output-file`, либо `<очищенный videoId>---<uuid>.mp3` (`clearFileName()` + `uuidv4()`).
     - Вызывает `downloadFile(translateResult.urlOrError, OUTPUT_DIR/filename, ...)`.
     - `axios` открывает GET-поток по указанному URL (обычно Яндекс-S3 или прокси-URL), данные пишутся прямо в файл.
   - Для субтитров:
     - CLI ищет в `translateResult.subsOrError` элемент с `language === RESPONSE_LANG`.
     - Формирует имя файла с расширением `.json` или `.srt` (в зависимости от флагов `--subs-srt`/`--subtitles-srt`).
     - Вызывает `downloadFile(subOnReqLang.url, OUTPUT_DIR/filename, ...)`.
     - Если формат `.srt`, скачанный JSON конвертируется в SRT.

---

## 4. Куда и зачем ходит `vot-cli` (сводка по доменам)

1. **`api.browser.yandex.ru`** — основной и обязательный эндпоинт:
   - `/video-translation/translate` — запрос перевода видео (получить URL аудио);
   - `/video-subtitles/get-subtitles` — запрос субтитров.
   - Зачем: получить от Яндекса сам перевод (аудио/сабы) по произвольной ссылке на поддерживаемых видеосервисах.
2. **Домен, указанный в `translateResponse.url` / `translatedUrl` / `url` субтитров** (как правило, Яндекс S3 / CDN, возможно через собственные прокси):
   - GET по прямому URL для скачивания аудио либо JSON с субтитрами.
   - Зачем: физически сохранить результат перевода на диск пользователя.
3. **`coursehunter.net`** (опционально, только если входная ссылка — CourseHunter):
   - `https://coursehunter.net/course/{videoId}` — парсинг HTML для получения `course_id`.
   - `https://coursehunter.net/api/v1/course/{courseId}/lessons` — JSON для сопоставления `lessonId` и файла `file` (видео-URL).
   - Зачем: превратить абстрактную ссылку на урок курса в прямой URL на видео, который затем передаётся в Яндекс.
4. **Прокси-сервер (любой HTTP/HTTPS прокси)**:
   - Настраивается пользователем через `--proxy`; CLI не содержит списка своих прокси-доменов и не навязывает конкретный сервер.
   - Зачем: дать возможность пользователю скрыть реальный IP/регион или обойти блокировки доступа к `api.browser.yandex.ru`.

---

## 5. Итоговые выводы

1. `vot-cli` — тонкий CLI-клиент поверх внутренних (частных) API Яндекс.Браузера видеоперевода, который напрямую общается с `api.browser.yandex.ru` по HTTPS, подделывая UA и криптоподписи, и получает оттуда URL готового перевода и субтитров.
2. Все сетевые операции реализованы через `axios` и встроенный `fetch`, без дополнительных бэкендов/прокси со стороны автора (кроме тех, которые пользователь может явно задать через `--proxy`).
3. Помимо Яндекса, утилита умеет вытягивать реальные видео-URL с CourseHunter (через его публичное HTML/JSON API), чтобы затем отдавать их Яндексу на перевод.
4. Вся бизнес-логика CLI сводится к: нормализовать ссылку → подготовить protobuf-запрос → опросить Яндекс-видеоперевод → вывести пользователю URL/статус → при необходимости скачать и, опционально, конвертировать результат в аудио или SRT-файл.