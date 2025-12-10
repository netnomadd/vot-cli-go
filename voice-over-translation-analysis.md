# Анализ репозитория `ilyhalight/voice-over-translation`

## 1. Общий принцип работы расширения

1. Расширение представляет собой userscript (Tampermonkey/Violentmonkey и др.), который встраивается на множество видеосайтов (YouTube, Twitch, VK, Coursera, TikTok, локальные MP4/WebM и т.д.).
2. Скрипт отслеживает появление `<video>`-элементов через `VideoObserver` и для каждого найденного видео создаёт объект `VideoHandler` (`src/index.js`).
3. `VideoHandler` собирает информацию о видео (ID, URL, длительность, хост, доступные сабы и т.п.) с помощью функций из внешней библиотеки `@vot.js/ext` (`getVideoData`, `getService`, `getVideoID`) и затем инициирует перевод через клиент `VOTClient` / `VOTWorkerClient` (также из `@vot.js/ext`).
4. В зависимости от настроек и типа видео (запись/стрим) `VideoHandler`:
   - запрашивает перевод видео/стрима у серверов Яндекса через VOT-клиент,
   - при необходимости скачивает оригинальную аудиодорожку (через `AudioDownloader`) и догружает её на сервер перевода,
   - получает URL переведённого аудио или HLS-потока,
   - подключает свой аудиоплеер (на базе `Chaimu`) и синхронизирует его с исходным видео (громкость, пауза/плей, стриминг HLS и т.п.),
   - получает и отображает субтитры (Яндекс + сабы сайта).
5. Весь сетевой трафик организован вокруг вспомогательной обёртки `GM_fetch` (`src/utils/gm.ts`), которая:
   - сначала пытается использовать обычный `fetch`,
   - при ошибке или при обращении к «проблемным» доменам (например, `api.browser.yandex.ru`) принудительно переходит на `GM_xmlhttpRequest` Tampermonkey, обходя CORS.
6. Локализация интерфейса (надписи в меню, ошибки и т.п.) динамически подгружается из самого GitHub-репозитория через `LocalizationProvider` (`src/localization/localizationProvider.ts`).

---

## 2. Основные точки входа и сетевые обёртки

### 2.1. `GM_fetch` — единая точка HTTP-запросов

Файл: `src/utils/gm.ts`

- Функция `GM_fetch(url, opts)` принимает все HTTP-запросы из кода (в т.ч. передаётся как `fetchFn` во внешние клиенты `VOTClient`, `Chaimu`, `getVideoData` и т.п.).
- Особое поведение:
  - Если `url` — строка и содержит `"api.browser.yandex.ru"`, кидает ошибку `"Preventing yandex cors"`, чтобы форсировать переход на GM-запрос.
  - В блоке `catch` логирует переход на `GM_xmlhttpRequest` и формирует заголовки через `getHeaders` (`src/utils/utils.ts`).
  - `GM_xmlhttpRequest` вызывается с:
    - методом (`GET`/`POST`/`HEAD` и т.д.),
    - `responseType: "blob"`,
    - произвольным `data` (тело запроса),
    - `timeout` (по умолчанию до 15000 мс, либо 0 для некоторых вызовов).
  - Затем GM-ответ оборачивается в экземпляр `Response`, чтобы остальной код работал так, как с обычным `fetch`.
- Таким образом, **фактическая отправка сетевых запросов** в большинстве случаев идёт через `GM_xmlhttpRequest`, а не через нативный `fetch`, что даёт полный доступ к кросс-доменным ресурсам в рамках Tampermonkey.

### 2.2. Конфигурация доменов и коннектов

Файлы:
- `src/config/config.js`
- `src/headers.json`

`src/config/config.js` определяет ключевые хосты и URL:

- `const workerHost = "api.browser.yandex.ru";` — основной хост яндекс-браузера, через который происходит перевод видео.
- `const m3u8ProxyHost = "media-proxy.toil.cc/v1/proxy/m3u8";` — прокси для HLS (`.m3u8`) потоков.
- `const proxyWorkerHost = "vot-worker.toil.cc";` — свой прокси-воркер, который может подменять прямой доступ к `api.browser.yandex.ru`.
- `const votBackendUrl = "https://vot.toil.cc/v1";` — собственный backend, который обеспечивает поддержку дополнительных сайтов/форматов.
- `const foswlyTranslateUrl = "https://translate.toil.cc/v2";` — API FOSWLY-переводчика (текстовый перевод и определение языка, см. ниже).
- `const detectRustServerUrl = "https://rust-server-531j.onrender.com/detect";` — альтернативный сервер определения языка на Rust.
- `const authServerUrl = "https://t2mc.toil.cc";` — домен авторизации/профиля пользователя.
- `const avatarServerUrl = "https://avatars.mds.yandex.net/get-yapic";` — источник аватарок Яндекса (используется во внешнем коде/интерфейсе аккаунта).
- `const repoPath = "ilyhalight/voice-over-translation";` и `const contentUrl = "https://raw.githubusercontent.com/${repoPath}";` — базовый URL для загрузки локализаций и прочего контента из GitHub.

`src/headers.json` содержит userscript-метаданные, которые Tampermonkey использует для выдачи разрешений:

- `"match"` — список поддерживаемых доменов/URL, на которых скрипт активен.
- `"connect"` — **список доменов, к которым разрешены кросс-доменные GM-запросы**, включая:
  - `yandex.ru`, `yandex.net`, множество поддоменов `disk.yandex.*` — доступ к API, файлам и ресурсам Яндекса.
  - `timeweb.cloud` — потенциальный временный или резервный хост бэкендов (в текущем коде прямых обращений нет, но домен разрешён).
  - `raw.githubusercontent.com` — скачивание локализаций и, при необходимости, других файлов из репозитория.
  - `vimeo.com`, `googlevideo.com`, `porntn.com` — прямой доступ к медиаресурсам отдельных хостов.
  - `toil.cc`, `deno.dev`, `onrender.com`, `workers.dev`, `speed.cloudflare.com` — домены инфраструктуры и вспомогательных сервисов.

---

## 3. Куда, как и зачем отправляются запросы

### 3.1. Яндекс-видеоперевод (`api.browser.yandex.ru`, `strm.yandex.ru`, `*.s3-private.mds.yandex.net`)

#### Куда

- Основной API видеоперевода Яндекса:
  - `api.browser.yandex.ru` — интерфейс браузера Яндекс, через который происходит запрос перевода видео/стрима.
  - `strm.yandex.ru` — домен HLS-потоков (переведённый звук для стримов).
  - `https://vtrans.s3-private.mds.yandex.net/tts/prod/...` — S3-хранилище с готовыми аудиофайлами перевода (tts/перевод видео).
  - `https://brosubs.s3-private.mds.yandex.net/vtrans/...` — хранилище субтитров.

#### Как

1. **Клиент `VOTClient` / `VOTWorkerClient`** (внешняя библиотека `@vot.js/ext`) создаётся в `VideoHandler.initVOTClient()` (`src/index.js`):
   - `hostVOT: votBackendUrl` — собственный backend `https://vot.toil.cc/v1`.
   - `host` — либо `workerHost` (`api.browser.yandex.ru`), либо `proxyWorkerHost` (`vot-worker.toil.cc`) в зависимости от настроек `translateProxyEnabled`.
   - `fetchFn: GM_fetch` — все его запросы идут через описанную выше обёртку.
   - `apiToken: this.data.account?.token` — токен авторизации (если пользователь залогинен через `authServerUrl`).
2. Перевод видео/стрима запускается через методы VOT-клиента:
   - `translateVideo` и `translateStream` вызываются из `VOTTranslationHandler` (`src/core/translationHandler.ts`).
   - Передаётся структура `videoData` (ID, URL, длительность, хост, язык, подсказки перевода и т.п.).
3. Для **стримов**:
   - `translateStreamImpl` получает от бэкенда описание HLS-потока.
   - Метод `setHLSSource(url)` в `VideoHandler` строит URL вида:
     ```ts
     const streamURL = `https://${this.data.m3u8ProxyHost}/?all=yes&origin=${encodeURIComponent("https://strm.yandex.ru")}&referer=${encodeURIComponent("https://strm.yandex.ru")}&url=${encodeURIComponent(url)}`;
     ```
   - То есть фактическое воспроизведение идёт **через собственный m3u8-прокси** (см. 3.3), но источник — `strm.yandex.ru`.
4. Для **обычных видео**:
   - Бэкенд Яндекса формирует `url` на аудиофайл перевода.
   - Перед проигрыванием `VideoHandler.updateTranslation` вызывает `validateAudioUrl`:
     - Для некоторых S3-URL (см. `isMultiMethodS3`) делает `HEAD` или `GET` с `range: bytes=0-0`, чтобы убедиться, что URL валиден.
     - В случае невалидности пробует сбросить `detectedLanguage` в `"auto"` и перезапросить перевод.
   - Далее `AudioPlayer` (`Chaimu`) начинает воспроизведение переведённого аудио.
5. Для **субтитров**:
   - `SubtitlesProcessor.getSubtitles` (`src/subtitles.js`) вызывает:
     ```ts
     const res = await client.getSubtitles({ videoData: { host, url, videoId, duration }, requestLang });
     ```
   - Ответ содержит набор субтитров с URL, зачастую указывающих на `brosubs.s3-private.mds.yandex.net`.
   - Эти URL затем скачиваются через `GM_fetch` и приводятся к единому JSON-формату.

#### Зачем

- Получение переведённой аудиодорожки (озвучки) и субтитров от **официального сервиса Яндекса** видеоперевода.
- Валидация корректности ссылок S3 и повторные запросы перевода при ошибках.
- Получение переводов как для записанных видео, так и для стримов.


### 3.2. Собственная инфраструктура FOSWLY / VOT (`*.toil.cc`, `*.workers.dev`, `*.deno.dev`)

#### Основные домены

- `vot-worker.toil.cc` (+ варианты `vot-worker-s1`, `vot-worker-s2`, `vot.deno.dev`, `vot-new.toil-dump.workers.dev` — перечислены в README) — **прокси-воркеры** между браузером и API Яндекса.
- `https://vot.toil.cc/v1` — VOT-backend (см. отдельный репозиторий `FOSWLY/vot-backend`).
- `https://media-proxy.toil.cc/v1/proxy/m3u8` — media-proxy (см. `FOSWLY/media-proxy`), проксирующий HLS-потоки.
- `https://translate.toil.cc/v2` — свой бэкенд текстового перевода (`FOSWLY/translate-backend`).
- `https://t2mc.toil.cc` — домен авторизации и профиля пользователя.
- `https://votstatus.toil.cc`, `https://votstats.toil.cc` — страницы статуса/статистики (используются пользователем вручную, скрипт к ним сам не обращается).

#### Как используются

1. **Прокси для видеоперевода**:
   - В `VideoHandler.initVOTClient()` поле `host` выбирается так:
     ```js
     host: this.data.translateProxyEnabled
       ? this.data.proxyWorkerHost
       : workerHost,
     ```
   - Пользовательские настройки и автоопределение страны (см. 3.4) могут включать режимы:
     - прямой доступ к `api.browser.yandex.ru`,
     - принудительный прокси через `vot-worker.toil.cc`.
2. **Media-proxy для HLS**:
   - Для стримов и HLS-аудио используется `m3u8ProxyHost`:
     - См. `setHLSSource()` — создаётся ссылку на `https://media-proxy.toil.cc/v1/proxy/m3u8` с параметрами `origin`, `referer`, `url`.
   - В результате браузер получает поток от собственного прокси, а не напрямую от `strm.yandex.ru`.
3. **VOT-backend**:
   - `votBackendUrl` передаётся в `VOTClient` как `hostVOT`. Код самого клиента вынесен в отдельный пакет `@vot.js/ext` / `vot-backend`.
   - Назначение бэкенда: поддержка дополнительных сайтов/форматов, которые не переводятся стандартным API Яндекса (подробности в репозитории `FOSWLY/vot-backend`).
4. **FOSWLY Translate API** (текстовый перевод):
   - Файл: `src/utils/translateApis.ts`.
   - Используется для:
     - перевода UI-сообщений об ошибках на язык интерфейса пользователя,
     - определения языка текста (детекция языка названия/описания видео и т.п.).
   - Реализован класс `FOSWLYTranslateAPI`, который ходит на:
     - `POST /translate` — множественный перевод массива строк,
     - `GET /translate?text=...&lang=...&service=...` — одиночный перевод,
     - `GET /detect?text=...&service=...` — определение языка.
   - Параметр `service` соответствует `ClientType.TranslationService` и может быть `"yandexbrowser"` или `"msedge"` — бэкенд маршрутизирует запросы на соответствующие движки.
5. **Rust-сервер детекции языка**:
   - `detectRustServerUrl = "https://rust-server-531j.onrender.com/detect"`.
   - В `translateApis.ts` объект `RustServerAPI` делает `POST` с телом `text` и возвращает строку кода языка.
   - Используется как дополнительный вариант сервиса детекции.
6. **Авторизация** (`t2mc.toil.cc`):
   - Файл: `src/core/auth.ts`.
   - При открытии страниц на домене `authServerUrl` выполняется `initAuth()`:
     - Если `pathname === "/auth/callback"` — читается фрагмент URL (`#access_token=...&expires_in=...`), сохраняется токен и время истечения в `votStorage` под ключом `account`.
     - Если `pathname === "/my/profile"` — используется глобальный объект `_userData` (заполняется самим сайтом) для чтения `avatar_id` и `username`, которые добавляются к уже сохранённому аккаунту.
   - Этот токен затем передаётся в `VOTClient` (`apiToken`) и может разблокировать дополнительные функции (например, "Lively Voice").

#### Зачем

- Снять ограничения CORS, гео- и сетевые блокировки, а также реализовать обход проблем с доступом к API Яндекса.
- Поддержать форматы/сайты, которые не понимает стандартный API Яндекса.
- Обеспечить автономный текстовый перевод/детекцию языка независимо от встроенных браузерных переводчиков.
- Добавить авторизацию пользователей и дополнительные функции (например, улучшенные голоса).


### 3.3. GitHub (`raw.githubusercontent.com`)

Файл: `src/localization/localizationProvider.ts`

#### Куда

- Базовый URL: `localizationUrl = `${contentUrl}/${REPO_BRANCH}/src/localization``.
  - `contentUrl` = `https://raw.githubusercontent.com/ilyhalight/voice-over-translation`.
- Конкретные запросы:
  - `GET {localizationUrl}/hashes.json` — карта хэшей локалей.
  - `GET {localizationUrl}/locales/{lang}.json` — файл локализации для конкретного языка.

#### Как

1. При запуске `LocalizationProvider.update()`:
   - Проверяет, не устарели ли локали (TTL = 7200 секунд) и совпадает ли сохранённый язык с текущим.
   - Если нужно обновить — через `GM_fetch` запрашивает `hashes.json`.
   - Сравнивает хэш текущего языка с сохранённым `localeHash` — если отличаются, подгружает соответствующий JSON локализации.
2. Файлы сохраняются в `votStorage` как сырые JSON-строки, затем расплющиваются в плоский объект (`toFlatObj` из `src/utils/utils.ts`).

#### Зачем

- Обновлять переводы интерфейса расширения (тексты кнопок, подсказок, ошибок) **без обновления самого userscript-файла**.
- Позволить использовать единый репозиторий как источник локализаций для разных клиентов (userscript, CLI, другие проекты).


### 3.4. Определение страны пользователя (`speed.cloudflare.com`)

Файл: `src/index.js`

#### Куда

- `https://speed.cloudflare.com/meta`.

#### Как

- В `VideoHandler.init()` при первой инициализации (если `countryCode` ещё не определён):
  - вызывается `GM_fetch("https://speed.cloudflare.com/meta", { timeout: 7000 })`,
  - из ответа JSON читается поле `country` и сохраняется в глобальную переменную `countryCode`.

- Затем, если страна попадает в `proxyOnlyCountries = ["UA", "LV", "LT"]` и включён режим `translateProxyEnabledDefault`, перевод автоматически переводится в режим полного прокси (`translateProxyEnabled = 2`).

#### Зачем

- Автоматически решать, нужно ли **обязательно использовать прокси-серверы** для перевода (например, в регионах, где прямой доступ к API Яндекса невозможен/нестабилен).


### 3.5. Хосты видеосайтов и их API

Часть доступа к сторонним доменам осуществляется через внешнюю библиотеку `@vot.js/ext`, но в самом репозитории можно выделить следующие моменты:

1. **YouTube и Google Video**:
   - `SubtitlesProcessor.fetchSubtitles()` подставляет дополнительные параметры (`poToken`, `deviceParams`) к URL субтитров YouTube и скачивает их через `GM_fetch`.
   - Для стримов и превью видео используются утилиты `YoutubeHelper` (пакет `@vot.js/ext/helpers/youtube`).
2. **Собственные сабы сайта**:
   - В `SubtitlesProcessor.getSubtitles` аргумент `videoData.subtitles` может содержать список чужих субтитров (VK, другие сайты), к которым затем обращаются через `GM_fetch`.
   - Для VK-субтитров применяется очистка HTML (`cleanJsonSubtitles`).
3. **Хосты-источники аудио**:
   - Механизм `AudioDownloader` (`src/audioDownloader/index.ts` + стратегии в `/src/audioDownloader/strategies`) работает через скрытый iframe и перехват внутренних Web API вызовов, чтобы получить прямые URL аудио/видео с сайта-источника.
   - Сами сетевые запросы в стратегиях выполняются в коде `@vot.js` и браузера; в данном репозитории реализована только оболочка и обмен сообщениями между основным миром и iframe (`iframeConnector`, `shared.ts`).

#### Зачем

- Читать субтитры с сайта (YouTube, VK, и др.) и дополнительно к яндексовским использовать их для перевода/отображения.
- Получать оригинальный аудиопоток с видеосайта, если Яндекс требует "загрузить аудио" для перевода.


### 3.6. Прочие домены из `connect`-списка

Некоторые домены разрешены в `src/headers.json`, но явно не используются в текущем коде репозитория:

- `timeweb.cloud` — вероятно, используется в смежных проектах или был хостом бэкенда ранее.
- Различные поддомены `toil.cc`, `deno.dev`, `workers.dev` — задействованы в других окружениях `vot-worker` и `media-proxy` (их конкретное использование можно увидеть в репозиториях `FOSWLY/vot-worker` и `FOSWLY/media-proxy`).
- `porntn.com`, `vimeo.com`, `googlevideo.com` — низкоуровневые источники медиа/файлов для отдельных поддерживаемых сайтов, доступ к их URL обеспечивается через библиотеки `@vot.js` и/или стратегии аудиодоступа.

---

## 4. Взаимодействие компонентов при переводе видео

Ниже — последовательность действий при типичном сценарии перевода обычного (не стрима) YouTube-видео.

1. **Обнаружение видео**:
   - `VideoObserver` следит за DOM и при добавлении `<video>`-элемента генерирует событие `onVideoAdded`.
   - Обработчик в `main()` (`src/index.js`) находит подходящий `site` через `getService()` (`@vot.js/ext/utils/videoData`) и создаёт `new VideoHandler(video, container, site)`.
2. **Сбор метаданных**:
   - `VideoHandler.videoData = await videoHandler.getVideoData()` → `VOTVideoManager.getVideoData()` (`src/core/videoManager.ts`).
   - `getVideoData` (из `@vot.js/ext`) получает ID, URL, длительность, хост, начальный язык и доступные сабы.
   - Если язык не известен, берётся заголовок/описание видео, очищается от мусора (`cleanText`), и вызывается `detect(text)` из `translateApis.ts`:
     - либо через `FOSWLYTranslateAPI.detect` (детекция по API Яндекс/Edge),
     - либо через `RustServerAPI.detect` (сервер на onrender.com).
3. **Инициализация UI и клиента**:
   - `VideoHandler.init()` читает настройки из `votStorage` (localStorage / GM_storage), инициализирует UI (`UIManager`), субтитры (`SubtitlesWidget`), аудиоплеер (`Chaimu`) и клиента `VOTClient`.
   - В процессе может быть выполнен запрос к `speed.cloudflare.com/meta` для определения страны и выбора режима прокси.
4. **Запрос перевода**:
   - При нажатии кнопки (или автостарте) вызывается `VideoHandler.translateFunc(...)`.
   - Создаётся ключ кеша, проверяется наличие сохранённого результата в `CacheManager`.
   - При отсутствии кеша вызывается `translationHandler.translateVideoImpl(...)`:
     - `votClient.translateVideo({ videoData, requestLang, responseLang, translationHelp, extraOpts, shouldSendFailedAudio })`.
   - Бэкенд либо сразу возвращает готовый `url` перевода, либо статус ожидания/загрузки аудио.
5. **Загрузка оригинального аудио (если требуется)**:
   - Если `res.status === VideoTranslationStatus.AUDIO_REQUESTED` и сайт — YouTube (`isYouTubeHosts()`), запускается `audioDownloader.runAudioDownload(videoId, translationId, signal)`.
   - `AudioDownloader` через стратегию (по умолчанию `WEB_API_GET_ALL_GENERATING_URLS_DATA_FROM_IFRAME`) и iframe получает по частям бинарные данные аудио, диспатчит события `downloadedAudio`/`downloadedPartialAudio`.
   - Обработчики в `VOTTranslationHandler` отправляют эти части на сервер переводчика через `votClient.requestVtransAudio(videoUrl, translationId, { audioFile, fileId, chunkId }, meta)`.
   - После окончания загрузки повторно вызывается `translateVideoImpl` для получения конечного URL перевода.
6. **Воспроизведение переведённого аудио**:
   - `VideoHandler.updateTranslation(audioUrl)`:
     - при необходимости валидирует/чинит URL (`validateAudioUrl` c пробным запросом),
     - при включённом жёстком прокси для S3 подменяет домен на `proxyWorkerHost` (`proxifyAudio`).
   - Настраивает `audioPlayer`, включает автоматическую подстройку громкости (`setupAudioSettings`) и обновляет состояние UI.
7. **Работа с субтитрами**:
   - `loadSubtitles()` запрашивает и кэширует субтитры через `SubtitlesProcessor.getSubtitles(this.votClient, this.videoData)`.
   - При смене активных сабов, `SubtitlesWidget` получает контент и отрисовывает его поверх видео, при желании подсвечивая слова и отображая всплывающий перевод отдельного слова (через `translate()` на FOSWLY API).

---

## 5. Краткие выводы с точки зрения сетевого поведения

1. **Основной функционал перевода видео и стримов** завязан на API Яндекс.Браузера и его облачную инфраструктуру (включая S3 и HLS), но почти всегда работает **через собственные прокси-сервера проекта** (`vot-worker`, `media-proxy`, `vot-backend`), что позволяет обходить CORS и региональные ограничения.
2. **Текстовые переводы и детекция языка** выполняются не напрямую через сторонние публичные API, а через собственный FOSWLY Translate Backend (`translate.toil.cc`), который, в свою очередь, использует YandexBrowser/MS Edge переводчики.
3. **Конфигурация и локализация интерфейса** подгружаются из GitHub (`raw.githubusercontent.com`) по HTTP, что позволяет обновлять языковые файлы без обновления userscript.
4. **Определение геолокации и выбор прокси-режима** происходят через запрос к `speed.cloudflare.com/meta`, после чего в странах с проблемным доступом к сервисам Яндекса принудительно включается прокси-перевод.
5. **Авторизация пользователя и расширенные функции** (например, Lively Voice) осуществляются через отдельный домен `t2mc.toil.cc`, куда пользователь переходит вручную; расширение только читает токен из URL и `_userData`, не отправляя обратно пароли или сессии.
6. Все сетевые запросы централизованно проходят через `GM_fetch`/`GM_xmlhttpRequest`, что делает поведение предсказуемым: любые упоминания доменов в коде почти всегда означают обращения именно через эту прослойку.
