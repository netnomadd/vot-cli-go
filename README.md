# ВНИМАНИЕ!

Утилита все еще в разработке.
Написана с помощью copilot с моим скромным участием в качестве куратора.

# vot-cli-go

Кроссплатформенная CLI-утилита на Go для перевода озвучки видео через сервисы Яндекса / FOSWLY. Поддерживаются два backend’а:

- **direct** — прямые запросы к `api.browser.yandex.ru` (динамический protobuf, HMAC-подпись заголовков);
- **worker** — запросы к worker-сервису (по умолчанию `https://vot-worker.toil.cc`, адрес можно переопределить в конфиге или через окружение), который проксирует обращение к Яндексу.

## Установка и сборка

### Готовые бинарники

Ожидаемый способ дистрибуции — выкладывать собранные бинарники в GitHub Releases этого репозитория.
Типичные файлы:
- `vot-linux-amd64`, `vot-linux-arm64`;
- `vot-windows-amd64.exe`;
- `vot-darwin-amd64`, `vot-darwin-arm64`.

После скачивания:
- сделайте файл исполняемым (`chmod +x vot-*` на Linux/macOS);
- положите его в каталог, находящийся в `PATH` (например, `/usr/local/bin` или `~/bin`).

### Сборка из исходников

```bash
# из корня репозитория
go build -o vot ./cmd/vot

# вариант без поддержки worker backend
go build -tags no_worker -o vot ./cmd/vot
```

Для кроссплатформенной сборки достаточно задать `GOOS`/`GOARCH`, например:

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o vot-linux-amd64 ./cmd/vot

# Linux arm64
GOOS=linux GOARCH=arm64 go build -o vot-linux-arm64 ./cmd/vot

# Windows amd64
GOOS=windows GOARCH=amd64 go build -o vot-windows-amd64.exe ./cmd/vot

# macOS amd64
GOOS=darwin GOARCH=amd64 go build -o vot-darwin-amd64 ./cmd/vot

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o vot-darwin-arm64 ./cmd/vot
```

Полученный бинарник `vot` можно положить в `$PATH`.

## Базовое использование

```bash
# минимальный пример: перевести видео на русский
vot translate --response-lang=ru "https://www.youtube.com/watch?v=..."

# указать язык оригинала явно
vot translate --request-lang=en --response-lang=ru "https://youtu.be/..."

# использовать worker backend
vot --backend=worker translate --response-lang=ru "https://youtu.be/..."

# переключить стиль голоса: tts вместо live
vot translate --voice-style=tts --response-lang=ru "https://youtu.be/..."

# скачать переведённое аудио в каталог (без -o/--outdir печатается только URL)
vot translate -o ./out --response-lang=ru "https://youtu.be/..."

# машинный JSON-итог (удобно для скриптов)
vot --json translate -o ./out --response-lang=ru "https://youtu.be/..."
```

Ключевые опции `translate`:

- `-s, --request-lang` — язык оригинала (пусто = автоопределение);
- `-t, --response-lang` — язык перевода (по умолчанию `ru`);
- `--voice-style` — `live` (по умолчанию, живые голоса) или `tts` (классический TTS);
- `--direct-url` — считать входные URL прямыми ссылками на медиа (mp4/webm);
- `--subs-url` — прямая ссылка на субтитры, которые передаются как подсказка для перевода;
- `-o, --outdir` — каталог для сохранения переведённого аудио; без флага CLI по-прежнему печатает только URL;
- `--outfile` — имя выходного файла (только для одного URL; иначе имя строится из title/`videoId`);
- `--no-title` — в имени файла использовать `videoId`/hash, не title из `yt-dlp`;
- `--keep-url` — при скачивании дополнительно печатать URL аудио (plain: `path<TAB>url`; в `--json` оба поля есть всегда);
- `--backend` — `direct` или `worker`;
- `--poll-interval` — интервал между запросами статуса: `0` (по умолчанию) = авто-режим по `remainingTime` из backend, `>=30` = фиксированный интервал в секундах;
- `--poll-attempts` — максимальное количество попыток опроса (`0` по умолчанию = без лимита попыток, до истечения `--max-wait`);
- `--max-wait` — максимальное общее время ожидания перевода (duration, например `30m`, `1h`; по умолчанию `30m`);
- `--allow-long-video` — разрешить обработку видео длиннее 4 часов (отключает локальную pre-check проверку лимита длительности);
- `--audio-requested-real-upload` — при `AUDIO_REQUESTED` сначала пытаться загрузить реальное аудио; по умолчанию выключено (поведение как в `vot.js`: `fail-audio-js` + empty audio);
- `--use-yt-dlp` — использовать локальный `yt-dlp` (если доступен в `PATH`) для получения прямых медиассылок и метаданных (в том числе длительности ролика);
- `--yt-dlp-use-direct-url` — при использовании `yt-dlp` передавать в backend прямой медиa-URL, а не оригинальный адрес страницы;
- `--yt-dlp-cookies` — путь к cookies-файлу, который будет передан в `yt-dlp` как `--cookies` (полезно для авторизованных/защищённых видео);
- `--yt-dlp-cookies-from-browser` — строка с описанием браузера/профиля для `yt-dlp --cookies-from-browser` (например, `firefox` или `chrome:Profile 1`).

Флаги верхнего уровня:

- `-c, --config` — путь к конфигурационному файлу;
- `-q, --silent` — "тихий" режим: в stdout только итог (URL или путь к файлу), сообщения об ошибках — в stderr;
- `--json` — один JSON-документ с итогами батча в stdout (без прогресса; перекрывает `--silent`);
- `-d, --debug` — подробный отладочный вывод;
- `--lang` — язык UI-сообщений (`ru`, `en` и т.п.).

## Конфигурационный файл

По умолчанию конфиг ищется по пути:

- Linux/macOS: `$XDG_CONFIG_HOME/vot-cli/config.json` (обычно `~/.config/vot-cli/config.json`);
- Windows: `%APPDATA%\\vot-cli\\config.json`.

Путь можно переопределить флагом `--config`.

Структура `config.json`:

```json
{
  "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ... YaBrowser/25.4.0.0 Safari/537.36",
  "yandex_hmac_key": "bt8xH3VOlb4mqf0nqAibnDOoiPlXsisf",
  "yandex_token": "ya-oauth-token-for-lively-voice-if-needed",
  "worker_url": "https://vot-worker.toil.cc",
  "default_response_lang": "ru",
  "use_yt_dlp": true,
  "yt_dlp_use_direct_url": true,
  "yt_dlp_cookies": "/path/to/cookies.txt",
  "yt_dlp_cookies_from_browser": "firefox",
  "source_rules": [
    {
      "patterns": [
        "(?i)^https?://example.com/",
        "(?i)^https?://alt.example.com/"
      ],
      "use_yt_dlp": true,
      "yt_dlp_use_direct_url": false,
      "request_lang": "de",
      "backend": "worker",
      "voice_style": "tts",
      "rewrite": [
        {
          "pattern": "(?i)^https?://alt.example.com/(.*)",
          "replace": "https://example.com/$1"
        }
      ]
    }
  ]
}
```

- `user_agent` — User-Agent, который будет использован в запросах к Яндексу/worker’у;
- `yandex_hmac_key` — ключ для HMAC-подписи `Sec-*` заголовков (по умолчанию прошит в бинарник);
- `yandex_token` — OAuth-токен Яндекса, используемый для живых голосов (Lively Voice), если задан;
- `worker_url` — базовый URL worker backend’а (только схема + host, без path/query/fragment), если нужно использовать не `https://vot-worker.toil.cc`, а другой адрес;
- `default_response_lang` — язык перевода по умолчанию, если флаг `--response-lang` не указан (по умолчанию `ru` в бинарнике; это поле позволяет сменить его, не меняя CLI-аргументы);
- `use_yt_dlp` — при `true` CLI может использовать локально установленный `yt-dlp` (если он найден в `PATH`) для получения прямых медиассылок/метаданных;
- `yt_dlp_use_direct_url` — при `true` backends получают уже «распакованный» прямой URL от `yt-dlp`, при `false` — исходный URL (напр. страница YouTube);
- `yt_dlp_cookies` — путь к cookies-файлу, который будет передан в `yt-dlp` как `--cookies` (для сайтов, где без авторизации/куков доступ ограничен);
- `yt_dlp_cookies_from_browser` — значение для `yt-dlp --cookies-from-browser` (например, `firefox` или `chrome:Profile 1`), если удобнее брать cookies из браузера;
- `source_rules` — (опционально) массив правил для конкретных источников: каждое правило содержит регулярное выражение `pattern` или массив `patterns` (для нескольких URL), флаги `use_yt_dlp`/`yt_dlp_use_direct_url` и, при необходимости, поля `request_lang`/`backend`/`voice_style`/`yt_dlp_cookies`/`yt_dlp_cookies_from_browser` и массив переписывателей `rewrite`, позволяя тонко настраивать поведение для разных сайтов без перекомпиляции бинарника.

### Правила источников (`source_rules`)

Правила источников применяются **поверх** базовых настроек: сначала берутся значения из конфига (`use_yt_dlp`, `yt_dlp_use_direct_url`, `backend`, `request_lang`, `yt_dlp_cookies`, `yt_dlp_cookies_from_browser`), затем для каждого URL последовательно применяются регулярные выражения из `source_rules`, а уже **поверх всего** работают явные CLI-флаги (которые всегда имеют приоритет). Для `voice_style` допустимы значения `live` и `tts`; при неверном значении из правила CLI молча откатывается к `live` (с записью в debug-лог при `--debug`).
Одно правило может содержать одиночный `pattern` или массив `patterns` для разных URL, а массив `rewrite` внутри правила задаёт дополнительные regexp-переписывания (`pattern`/`replace` в терминах Go `regexp.ReplaceAllString`) и применяется к уже совпавшим URL перед передачей их в yt-dlp/backend.
В бинарник также зашит небольшой набор дефолтных правил для известных источников (YouTube, Invidious/Piped, ZDF), который включён до пользовательских правил; за счёт этого вы можете переопределять встроенное поведение добавлением своих записей в `source_rules`.
Если какое-то регулярное выражение в `source_rules` не компилируется, оно тихо игнорируется и не ломает запуск утилиты.

### Переопределение через переменные окружения

Если заданы следующие переменные окружения, они **переопределяют** значения из `config.json`:

- `VOT_USER_AGENT` → `user_agent`;
- `VOT_YANDEX_HMAC_KEY` → `yandex_hmac_key`;
- `VOT_YANDEX_TOKEN` → `yandex_token`;
- `VOT_WORKER_URL` → `worker_url`.

Пример:

```bash
export VOT_USER_AGENT="Mozilla/5.0 ... YaBrowser/25.6.0.0 ..."
export VOT_YANDEX_TOKEN="ya-0.AQA..."

vot translate --response-lang=ru "https://youtu.be/..."
```

## Интеграция с `yt-dlp`

При включённой опции `use_yt_dlp` (через конфиг или флаг `--use-yt-dlp`) утилита, если находит `yt-dlp` в `PATH`, сначала вызывает его для переданного URL.
Из вывода берутся как минимум прямой медиa-URL и длительность ролика; длительность дополнительно отправляется в backend (direct/worker), что помогает сервису точнее оценивать объём работы и таймауты.
Если включён `yt_dlp_use_direct_url` (или флаг `--yt-dlp-use-direct-url`), в запрос к backend уходит уже «распакованный» медиa-URL; иначе — исходный адрес страницы (YouTube, Invidious и т.п.).
Если `yt-dlp` не найден или завершается с ошибкой, утилита продолжает работу как без него.
Если `yt-dlp` падает на YouTube/подобных сайтах с сообщениями про необходимость авторизации, можно задать cookies либо через флаги `--yt-dlp-cookies`/`--yt-dlp-cookies-from-browser`, либо через одноимённые поля в конфиге/`source_rules`.
Обратите внимание, что backend’ы `direct` и `worker` не гарантируют поддержку любых прямых медиассылок: часть URL'ов может не приниматься или обрабатываться с ошибкой.

## Отличия backend'ов `direct` и `worker`

**direct**:
- утилита сама ходит к `https://api.browser.yandex.ru` и собирает protobuf-запросы и заголовки, имитируя браузер;
- критичен к актуальности `user_agent` и `yandex_hmac_key` — при изменениях протокола/заголовков на стороне Яндекса возможны ошибки 4xx/5xx;
- меньше внешних зависимостей (нет дополнительного сервиса), но больше «ломается» при изменениях API Яндекса и может чаще упираться в лимиты/блокировки.

**worker**:
- утилита общается с worker-сервисом (по умолчанию `https://vot-worker.toil.cc`) по HTTP+JSON;
- protobuf-пакеты и `Sec-*` заголовки формируются уже на стороне worker’а, CLI передаёт только параметры (URL, языки, стиль голоса и т.п.);
- `user_agent` и `yandex_hmac_key` по-прежнему используются для генерации заголовков, но детали протокола с Яндексом инкапсулированы в worker’е;
- удобен, если не хочется следить за изменениями API Яндекса: интерфейс CLI остаётся стабильным, а обновления логики происходят на стороне worker-сервиса.

В нормальной ситуации оба backend’а должны выдавать сопоставимый результат; если один из них начинает стабильно отвечать ошибками (например, 403/429), можно переключиться на другой флагом `--backend`. 

## Локализация

Язык UI выбирается так:

1. Явный флаг `--lang` (`--lang=ru` или `--lang en` и т.п.);
2. Переменная окружения `VOT_LANG` (например, `VOT_LANG=ru`);
3. По умолчанию — английский.

В данный момент локализованы основные сообщения и help-тексты для `en` и `ru`.

## Режим `--silent`

При использовании `--silent` утилита печатает в `stdout` только итог: URL аудио либо путь к скачанному файлу (если задан `--outdir`/`--outfile`). Все диагностические сообщения и ошибки уходят в `stderr`.

## Режим `--json`

Флаг `--json` выводит в `stdout` один JSON-документ по всему батчу URL (без строк прогресса). Он удобнее `--silent` для скриптов: есть `ok`, `summary` и массив `results` с полями вроде `input`, `status`, `url`, `outputPath`, `error`.

Пример успешного ответа:

```json
{
  "ok": true,
  "summary": { "total": 1, "success": 1, "failed": 0 },
  "results": [
    {
      "input": "https://www.youtube.com/watch?v=X98VPQCE_WI",
      "status": "success",
      "type": "audio",
      "videoId": "X98VPQCE_WI",
      "url": "https://example.com/audio.mp3",
      "outputPath": "/tmp/out/X98VPQCE_WI.mp3",
      "backend": "worker",
      "voiceStyle": "live"
    }
  ]
}
```

Код выхода: `0`, если ни один URL не завершился ошибкой; иначе `1`. При `--json` вместе с `--silent` ошибки не дублируются в stderr (остаются только в JSON).

## Скачивание аудио

По умолчанию CLI только печатает URL готовой дорожки. Чтобы сохранить файл:

- `-o, --outdir=PATH` — каталог назначения (создаётся при необходимости);
- `--outfile=NAME` — явное имя файла (только при одном входном URL; расширение `.mp3` добавится само, если его нет);
- `--no-title` — не запрашивать title через `yt-dlp` для имени файла;
- `--keep-url` — в plain-режиме печатать `путь<TAB>url`.

Имя файла без `--outfile`: title из `yt-dlp` (если доступен и не `--no-title`), иначе `videoId`/короткий hash от URL. При коллизии имён добавляется суффикс `-1`, `-2`, …

```bash
vot translate -o ./out --no-title --response-lang=ru "https://youtu.be/..."
vot translate -o ./out --outfile=track.mp3 --response-lang=ru "https://youtu.be/..."
```

## Поллинг статуса перевода

После отправки запроса к backend’у (`direct` или `worker`) утилита периодически опрашивает статус перевода по тем же идентификаторам запроса.
По умолчанию CLI работает в авто-режиме и, когда backend возвращает `remainingTime`, использует это значение как интервал до следующего запроса (с безопасными границами), чтобы не опрашивать слишком часто или слишком редко.
Если нужен ручной контроль, можно явно задать `--poll-interval` (секунды, минимум 30) — тогда включается фиксированный режим с постоянной задержкой между запросами.
Лимит попыток задаётся через `--poll-attempts`; значение по умолчанию `0` означает режим без лимита попыток (ограничение по времени задаёт `--max-wait`).
Флаг `--poll-attempts=0` включает режим без лимита попыток; в этом режиме завершение по времени контролируется `--max-wait` (по умолчанию `30m`).
Если после заданного лимита попыток или истечения `--max-wait` ссылка на аудио так и не появилась, команда завершается с ошибкой, в сообщении которой указывается, что перевод всё ещё в процессе или не удался.

## Разработка

Для локальной разработки доступны цели `Makefile`:

- `make build` — собрать бинарник `vot` из `./cmd/vot`;
- `make build-no-worker` — собрать `vot` без поддержки worker backend (`go build -tags no_worker ...`);
- `make test` — запустить `go test ./...`;
- `make fmt` — прогнать `gofmt` по исходникам в `cmd/` и `internal/`;
- `make vet` — запустить `go vet ./...` для базовой статической проверки кода;
- `make lint` — последовательно выполнить `fmt`, `vet` и `test`;
- `make generate-proto` — пересобрать `internal/yandexproto/yandex.pb.go` из `internal/yandexproto/yandex.proto` (требуются `protoc` и `protoc-gen-go`);
- `make ci` — локально повторить основные проверки CI (`vet` + `test`).

## Примеры

Перевод нескольких ссылок за один запуск (каждая ссылка будет обработана отдельно):

```bash
vot translate --response-lang=ru "https://youtu.be/...1" "https://youtu.be/...2"
```

Тихий режим, удобный для пайплайнов и скриптов:

```bash
vot --silent translate --response-lang=ru "https://youtu.be/..."  # только URL в stdout
```

Скачивание и JSON-итог:

```bash
vot translate -o ./out --no-title --response-lang=ru "https://youtu.be/..."
vot --json translate -o ./out --response-lang=ru "https://youtu.be/..."
```

Использование worker backend с конфигом и локализацией UI:

```bash
VOT_LANG=ru vot --backend=worker --config ~/.config/vot-cli/config.json translate --response-lang=ru "https://youtu.be/..."
```

### Смена адреса worker backend

Если публичный worker переехал на другой домен или вы подняли свой экземпляр, достаточно переопределить базовый URL:

```json
{
  "worker_url": "https://worker.example.com"
}
```

или:

```bash
export VOT_WORKER_URL="https://worker.example.com"
vot --backend=worker translate --response-lang=ru "https://youtu.be/..."
```

CLI ожидает здесь только схему и host; пути вида `https://worker.example.com/api` не поддерживаются, потому что нужные endpoint’ы он добавляет сам.

### Сборка без поддержки worker

Если нужна сборка, не зависящая от внешнего worker backend, можно отключить этот код на этапе компиляции:

```bash
go build -tags no_worker -o vot ./cmd/vot
```

или:

```bash
make build-no-worker
```

В такой сборке backend `worker` недоступен: явный `--backend=worker` и пользовательские `source_rules` с `backend=worker` будут завершаться понятной ошибкой. Встроенные `source_rules` при этом больше не переключают YouTube / Invidious / ZDF на worker автоматически.

## Примеры конфигов

### Минимальный конфиг (direct backend)

```json
{
  "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ... YaBrowser/25.4.0.0 Safari/537.36",
  "default_response_lang": "ru"
}
```

Подойдёт, если вы используете только `direct` и не хотите трогать Yandex OAuth / HMAC-ключ в явном виде (используется значение по умолчанию из бинарника).

### Worker + yt-dlp по умолчанию

```json
{
  "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ... YaBrowser/25.4.0.0 Safari/537.36",
  "yandex_hmac_key": "bt8xH3VOlb4mqf0nqAibnDOoiPlXsisf",
  "yandex_token": "ya-0.AQA...",
  "default_response_lang": "ru",
  "use_yt_dlp": true,
  "yt_dlp_use_direct_url": false,
  "yt_dlp_cookies": "/path/to/cookies.txt",
  "yt_dlp_cookies_from_browser": "firefox"
}
```

В таком варианте `vot` по умолчанию использует `yt-dlp` (если он установлен) для получения метаданных и длительности, но в backend отправляется оригинальный URL страницы, а не прямой медиa-URL.

### Пример с `source_rules` для популярных сайтов

```json
{
  "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ... YaBrowser/25.4.0.0 Safari/537.36",
  "default_response_lang": "ru",
  "use_yt_dlp": true,
  "yt_dlp_use_direct_url": false,
  "source_rules": [
    {
      "pattern": "(?i)^https?://(www\\.)?youtube\\.com/watch",
      "use_yt_dlp": true,
      "yt_dlp_use_direct_url": false,
      "backend": "worker",
      "voice_style": "live",
      "yt_dlp_cookies_from_browser": "firefox"
    },
    {
      "pattern": "(?i)^https?://(www\\.)?youtu\\.be/",
      "use_yt_dlp": true,
      "yt_dlp_use_direct_url": false,
      "backend": "worker"
    },
    {
      "pattern": "(?i)^https?://(www\\.)?(invidious|piped)\\.",
      "use_yt_dlp": true,
      "yt_dlp_use_direct_url": false,
      "backend": "worker"
    },
    {
      "pattern": "(?i)^https?://www\\.zdf\\.de/play/",
      "use_yt_dlp": true,
      "yt_dlp_use_direct_url": true,
      "request_lang": "de",
      "backend": "worker",
      "voice_style": "tts"
    }
  ]
}
```

Эти правила:
- направляют все YouTube / Invidious / Piped ссылки через `yt-dlp` и backend `worker`;
- для ZDF сразу используют прямой URL от `yt-dlp`, фиксируют язык оригинала `de` и переключают голос на `tts`.

CLI-флаги при этом по-прежнему имеют приоритет и могут переопределить поведение из конфига.

## Сценарии для популярных сайтов

Ниже несколько типовых сценариев (предполагается, что конфиг похож на примеры выше).

### YouTube / Invidious / Piped (страничный URL)

```bash
vot translate -t ru "https://www.youtube.com/watch?v=..."
vot translate -t ru "https://invidious.example/watch?v=..."
```

- Правило из `source_rules` включает `yt-dlp`, получает длительность и, как правило, оставляет backend `worker`.
- Если backend не принимает прямой URL от `yt-dlp`, можно глобально или в правиле отключить `yt_dlp_use_direct_url`.

### ZDF (пример из wiki FOSWLY)

```bash
vot translate --debug "https://www.zdf.de/play/..."
```

- При срабатывании правила из `source_rules` URL предварительно обрабатывается `yt-dlp`, и в backend уходит уже прямой медиa-URL.
- Если сайт поменял схему URL или перестал поддерживаться, команда покажет более подробные debug-логи, по которым проще понять, на каком этапе произошёл сбой.

### Прямые mp4/webm ссылки

```bash
vot translate --direct-url -t ru "https://cdn.example.com/video.mp4"
```

- В этом режиме утилита не использует `yt-dlp` и считает, что URL уже указывает на готовый медиa-ресурс.
- Не все backend’ы принимают произвольные прямые ссылки; в случае отказа стоит вернуться к URL страницы и/или включить `yt-dlp`.

## Troubleshooting

### HTTP 403 / SESSION_REQUIRED / пустой URL от backend

- Убедитесь, что `user_agent` и `yandex_hmac_key` актуальны (сравните с рабочими примерами в репозиториях `vot-cli` / `vot.js`).
- Проверьте, не попадаете ли вы под блокировки/лимиты (частые запросы, VPN, нестабильный IP); иногда помогает смена IP или ожидание.
- Если ошибка приходит только от одного backend’а (`direct` или `worker`), попробуйте переключиться на другой флагом `--backend`.

### Перевод «завис» (много попыток поллинга, но без результата)

- Для длинных роликов попробуйте `--poll-attempts=0 --max-wait=45m` (или больше), чтобы не перезапускать команду вручную.
- Если не хотите безлимитный режим, увеличьте `--poll-attempts` и/или `--max-wait`.
- Включите `--debug`, чтобы видеть номер попытки и статус, возвращаемый backend’ом.
- Если после нескольких запусков ситуация не меняется для одного и того же URL, возможно, он сейчас не поддерживается сервисом перевода.

### Проблемы с `yt-dlp`

- Убедитесь, что `yt-dlp` установлен и доступен в `PATH` (`yt-dlp --version`).
- При подозрении на некорректный разбор сайта можно временно отключить интеграцию через конфиг (`"use_yt_dlp": false`).
- Предупреждения вида "No supported JavaScript runtime" относятся к самому `yt-dlp` и не блокируют работу `vot`, но могут приводить к отсутствию некоторых форматов.

### Неожиданные языки / голос

- Проверьте `default_response_lang` в конфиге и значение флага `--response-lang`.
- Убедитесь, что для URL не срабатывает правило из `source_rules`, которое меняет `request_lang` или `voice_style`.
- При отладке всегда полезно включать `--debug` — финальные параметры запроса (языки, backend, стиль голоса, использование `yt-dlp`) выводятся перед отправкой запроса.
