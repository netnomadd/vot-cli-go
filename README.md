# vot-cli-go

Кроссплатформенная CLI-утилита на Go для перевода озвучки видео через сервисы Яндекса / FOSWLY. Поддерживаются два backend’а:

- **direct** — прямые запросы к `api.browser.yandex.ru` (динамический protobuf, HMAC-подпись заголовков);
- **worker** — запросы к worker-сервису (по умолчанию `https://vot-worker.toil.cc`), который проксирует обращение к Яндексу.

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
```

Ключевые опции `translate`:

- `-s, --request-lang` — язык оригинала (пусто = автоопределение);
- `-t, --response-lang` — язык перевода (по умолчанию `ru`);
- `--voice-style` — `live` (по умолчанию, живые голоса) или `tts` (классический TTS);
- `--direct-url` — считать входные URL прямыми ссылками на медиа (mp4/webm);
- `--subs-url` — прямая ссылка на субтитры, которые передаются как подсказка для перевода;
- `--backend` — `direct` или `worker`;
- `--poll-interval` — интервал между запросами статуса (секунды, минимум 30);
- `--poll-attempts` — максимальное количество попыток опроса;
- `--use-yt-dlp` — использовать локальный `yt-dlp` (если доступен в `PATH`) для получения прямых медиассылок и метаданных (в том числе длительности ролика);
- `--yt-dlp-use-direct-url` — при использовании `yt-dlp` передавать в backend прямой медиa-URL, а не оригинальный адрес страницы.

Флаги верхнего уровня:

- `-c, --config` — путь к конфигурационному файлу;
- `-q, --silent` — "тихий" режим: в stdout выводятся только итоговые URL, сообщения об ошибках — в stderr;
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
  "default_response_lang": "ru",
  "use_yt_dlp": true,
  "yt_dlp_use_direct_url": true,
  "source_rules": [
    {
      "pattern": "(?i)^https?://www\\.zdf\\.de/play/",
      "use_yt_dlp": true,
      "yt_dlp_use_direct_url": true,
      "request_lang": "de",
      "backend": "worker"
    }
  ]
}
```

- `user_agent` — User-Agent, который будет использован в запросах к Яндексу/worker’у;
- `yandex_hmac_key` — ключ для HMAC-подписи `Sec-*` заголовков (по умолчанию прошит в бинарник);
- `yandex_token` — OAuth-токен Яндекса, используемый для живых голосов (Lively Voice), если задан;
- `default_response_lang` — язык перевода по умолчанию, если флаг `--response-lang` не указан (по умолчанию `ru` в бинарнике; это поле позволяет сменить его, не меняя CLI-аргументы);
- `use_yt_dlp` — при `true` CLI может использовать локально установленный `yt-dlp` (если он найден в `PATH`) для получения прямых медиассылок/метаданных;
- `yt_dlp_use_direct_url` — при `true` backends получают уже «распакованный» прямой URL от `yt-dlp`, при `false` — исходный URL (напр. страница YouTube);
- `source_rules` — (опционально) массив правил для конкретных источников: каждое правило содержит регулярное выражение `pattern`, флаги `use_yt_dlp`/`yt_dlp_use_direct_url` и, при необходимости, поля `request_lang`/`backend`/`voice_style`, позволяя тонко настраивать поведение для разных сайтов без перекомпиляции бинарника.

### Правила источников (`source_rules`)

Правила источников применяются **поверх** базовых настроек: сначала берутся значения из конфига (`use_yt_dlp`, `yt_dlp_use_direct_url`, `backend`, `request_lang`), затем для каждого URL последовательно применяются регулярные выражения из `source_rules`, а уже **поверх всего** работают явные CLI-флаги (которые всегда имеют приоритет). Для `voice_style` допустимы значения `live` и `tts`; при неверном значении из правила CLI молча откатывается к `live` (с записью в debug-лог при `--debug`).
В бинарник также зашит небольшой набор дефолтных правил для известных источников (YouTube, Invidious/Piped, ZDF), который включён до пользовательских правил; за счёт этого вы можете переопределять встроенное поведение добавлением своих записей в `source_rules`.
Если какое-то регулярное выражение в `source_rules` не компилируется, оно тихо игнорируется и не ломает запуск утилиты.

### Переопределение через переменные окружения

Если заданы следующие переменные окружения, они **переопределяют** значения из `config.json`:

- `VOT_USER_AGENT` → `user_agent`;
- `VOT_YANDEX_HMAC_KEY` → `yandex_hmac_key`;
- `VOT_YANDEX_TOKEN` → `yandex_token`.

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

При использовании `--silent` утилита печатает **только** итоговые URL аудио-перевода в `stdout`. Все диагностические сообщения и ошибки выводятся в `stderr`, что упрощает дальнейшее машинное парсинг/пайплайнинг вывода.

## Поллинг статуса перевода

После отправки запроса к backend’у (`direct` или `worker`) утилита периодически опрашивает статус перевода по тем же идентификаторам запроса.
Интервал и количество запросов настраиваются флагами `--poll-interval` (секунды, минимум 30) и `--poll-attempts` (по умолчанию 10); суммарное время ожидания примерно равно `interval * attempts`.
Если после заданного числа попыток ссылка на аудио так и не появилась, команда завершается с ошибкой, в сообщении которой указывается, что перевод всё ещё в процессе или не удался.

## Разработка

Для локальной разработки доступны цели `Makefile`:

- `make build` — собрать бинарник `vot` из `./cmd/vot`;
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

Использование worker backend с конфигом и локализацией UI:

```bash
VOT_LANG=ru vot --backend=worker --config ~/.config/vot-cli/config.json translate --response-lang=ru "https://youtu.be/..."
```
