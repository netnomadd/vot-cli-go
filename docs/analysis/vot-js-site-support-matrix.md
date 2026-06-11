# Анализ поддержки сайтов в `vot.js` и применимость для `vot-cli-go`

## Контекст

Цель: понять, какие сайты из `vot.js` можно поддержать в `vot-cli-go`:

1. без `yt-dlp`;
2. с `yt-dlp` (без отдельного helper-кода);
3. только через отдельный helper (site-specific интеграцию).

Источники в клоне `vot.js`:

- `packages/ext/src/data/sites.ts` — матрица поддерживаемых доменов/сервисов;
- `packages/ext/src/helpers/index.ts` — список helper-классов;
- отдельные helper-файлы в `packages/ext/src/helpers/*.ts`.

Дополнительно:

- в `voice-over-translation` есть локальная проверка `duration > 14400` (4 часа) в `src/core/videoManager.ts`;
- в `vot-worker` явного лимита длительности нет (прокси-роуты пересылают запросы на Яндекс без проверки duration).

---

## Что реально означает “поддерживается” в `vot.js/ext`

В `vot.js/ext` поддержка сайта обычно состоит из двух частей:

1. **детектор сайта** (entry в `sites.ts`);
2. **helper** (логика извлечения URL/метаданных/субтитров через DOM/API/GraphQL).

Для CLI (без браузерного DOM) это напрямую непереносимо: часть helper-логики завязана на страницу, плеер и внутренние API сайта.

---

## Категории для `vot-cli-go`

## 1) Можно поддержать без `yt-dlp` (высокая вероятность)

Эта группа ограничена. Реалистично:

- `youtube` / `youtu.be` (уже работает в проекте);
- прямые медиа-URL (`.mp4`, `.webm`) и часть публичных прямых ссылок;
- потенциально простые URL-rewrite сценарии (например, invidious/piped -> YouTube ID), если не требуется извлечение через DOM/API.

Почему список маленький:

- CLI не имеет доступа к DOM/JS runtime страницы;
- Яндекс backend часто ожидает либо оригинальную “понятную” ссылку, либо валидный прямой media URL + корректные метаданные.

---

## 2) Можно поддержать через `yt-dlp`, без отдельного helper

Это основной путь масштабирования CLI.

Подходит для сайтов, где `yt-dlp` умеет дать:

- рабочий media URL;
- duration;
- (опционально) audio download для `AUDIO_REQUESTED`.

Примеры из `vot.js`-набора, которые обычно имеют смысл пробовать через `yt-dlp`:

- YouTube / Invidious / Piped / PreserveTube
- Vimeo, Dailymotion, Twitch, Trovo
- X(Twitter), Facebook, Rumble, TikTok, Reddit
- Bilibili, Rutube, Dzen, Odysee, PeerTube
- Pornhub / Xvideos / Xhamster / Spankbang / Eporner / Rule34Video / ThisVid / WatchPornTo
- Archive, Newgrounds, Niconico, Kick, IMDB и др.

Практически это означает:

- добавлять правила в `source_rules`;
- включать `use_yt_dlp=true`;
- при необходимости подбирать `yt-dlp` cookies/profile.

---

## 3) Нужен отдельный helper (site-specific логика)

Это случаи, где в `vot.js` helper обращается к приватным/внутренним API, парсит нестандартные структуры, извлекает сабы/токены и т.д.

Типичные примеры из helper-кода:

- `udemy`, `coursera`, `datacamp`, `oraclelearn`, `deeplearningai`, `netacad`, `skilljar`
- `sap` (Kaltura multirequest), `epicgames`, `patreon`
- `yandexdisk`, `weverse`, `loom`, `kickstarter`
- `coursehunterLike`, `kodik`, `bannedvideo`, `artstation`, `mediafile`

Признаки “helper-only”:

- прямые вызовы API сайта (`fetch .../api/...`, GraphQL, internal endpoints);
- сборка `translationHelp` (`video_file_url` / `subtitles_file_url`);
- логика с нестандартными auth/cookies/token flow;
- зависимость от player internals на странице.

---

## Вывод по стратегии для `vot-cli-go`

1. **Базовый масштабируемый путь**: `source_rules + yt-dlp`.
2. **Без `yt-dlp`** имеет смысл держать узкий “core” (YouTube + прямые media URL).
3. **Отдельные helper’ы** писать только для реально приоритетных платформ (ed-tech/enterprise), где `yt-dlp` не закрывает задачу.

---

## Можно ли использовать Lua для helper-ов?

Коротко: **технически да**, но как основная стратегия для `vot-cli-go` — спорно.

### Когда Lua уместен

- легковесные пользовательские правила извлечения/rewrites;
- быстрые экспериментальные парсеры без перекомпиляции Go-бинаря.

### Ограничения Lua-подхода

- нужен встраиваемый runtime (`gopher-lua`), sandbox и лимиты;
- сложнее отладка/типобезопасность по сравнению с Go;
- helper-ы для сложных сайтов (auth/API/media manifests) всё равно быстро растут в “полноценный код”, где Go обычно удобнее.

### Практическая рекомендация

- **основной путь**: helper-интерфейс на Go;
- Lua оставить как опциональный “rule/script layer” для простых кейсов;
- для сложных интеграций (Udemy/Coursera/SAP-like) писать нативные Go helper-ы.

---

## Предложение по этапам внедрения

1. Расширить `source_rules` под приоритетные сайты и стабилизировать `yt-dlp`-ветку.
2. Добавить helper SDK в `vot-cli-go` (интерфейс + lifecycle + diagnostics).
3. Реализовать 1-2 пилотных helper-а (например, `udemy` и `coursera`), затем оценить TCO.
4. Только после этого решать, нужен ли Lua-слой как дополнительная настройка.

