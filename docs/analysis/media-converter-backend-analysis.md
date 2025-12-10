# Анализ репозитория `FOSWLY/media-converter-backend`

## 1. Назначение и общий принцип работы

1. `media-converter-backend` — это HTTP-сервис (Bun + Elysia), который по ссылке на медиа (m3u8, mpd, m4a/m4v) ставит задачу в очередь, скачивает сегменты/файл, конвертирует их локально в `.mp4` (адаптированное под ffmpeg аудио/видео), сохраняет результат в файловой системе и отдаёт URL для скачивания.
2. Архитектурно сервис разделён на несколько слоёв:
   - HTTP-слой (`src/index.ts`, `src/controllers/*`) — принимает запросы `/v1/convert`, проверяет авторизацию, возвращает статусы конверсии и результат;
   - очередь/воркеры (`bullmq` + Redis в `src/worker.ts`, `src/jobs/*`) — асинхронно выполняют задачи по конвертации и очистке старых файлов;
   - конвертеры (`src/libs/converters/*`) — содержат логику скачивания сегментов/файлов и запуска `ffmpeg`/`MP4Box` для сборки финального `.mp4`;
   - хранилища/кеш (`src/database/*`, `src/cache/*`) — PostgreSQL (через `kysely`) + Redis (`ioredis`) для хранения состояния задач и временного кеша.
3. Внешние HTTP-запросы выполняются только для скачивания исходного медиа (через `fetchWithTimeout` в `libs/network.ts`), а вся конвертация делается локально с помощью CLI-инструментов `ffmpeg` и `MP4Box`.

---

## 2. HTTP-API и точка входа

### 2.1. Точка входа (`src/index.ts`)

```ts
const app = new Elysia({ prefix: "/v1" })
  .use(swagger(...))
  .use(HttpStatusCode())
  .use(cors(config.cors))
  .use(staticPlugin({ alwaysStatic: false }))
  .error({ /* маппинг кастомных ошибок */ })
  .onError(({ set, code, error, httpStatus }) => { ... })
  .use(healthController)
  .guard({
      headers: t.Object({ authorization: t.String({ title: "Authorization Basic token" }) }),
      beforeHandle: ({ headers: { authorization } }) => {
        if (!validateAuthToken(authorization)) return;
      },
    },
    (app) => app.use(convertController),
  )
  .listen({ port: config.server.port, hostname: config.server.hostname });

void (async () => { await initCleaner(); })();
```

- Базовый префикс всех маршрутов: `/v1`.
- Сервис поднимает:
  - Swagger-документацию по `/v1/docs`;
  - `staticPlugin` — раздачу статических файлов из `public` (включая результаты конвертации под `/media/...`);
  - контроллеры:
    - `/v1/health` — проверка здоровья;
    - защищённую группу `/v1/convert` — API конвертации (Basic-токен в заголовке `Authorization`).
- Обработчик ошибок мапит кастомные коды (`FAILED_CONVERT_MEDIA`, `UNSUPPORTED_MEDIA_TYPE_ERROR`, `UNAUTHORIZED_ERROR`, `VIDEO_FILE_COULDNT_FOUND`) на соответствующие HTTP-статусы.
- `initCleaner()` в фоне настраивает периодический job очистки старых файлов (через BullMQ Scheduler).

### 2.2. Контроллер `health` (`/v1/health`)

Файл: `src/controllers/health/index.ts`

```ts
export default new Elysia().group("/health", (app) =>
  app.get("/", () => ({ version: config.app.version, status: "ok" as const }), {...}),
);
```

- Входящих параметров нет.
- Возвращает версию и статус сервиса.

### 2.3. Контроллер `convert` (`/v1/convert`)

Файл: `src/controllers/convert/index.ts`

```ts
export default new Elysia().group("/convert", (app) =>
  app.use(convertModels).post(
    "/",
    async ({ body: { direction, file, extra_url } }) => { ... },
    { body: "convert", detail: {...} },
  ),
);
```

- Принимает в теле (`convertModels` описывает схему):
  - `direction` — строка вида `"m3u8-mp4"`, `"m4a-mp4"`, `"m4v-mp4"`, `"mpd-mp4"`;
  - `file` — либо URL (для m3u8/m4a/m4v), либо содержимое манифеста (для mpd допускается raw-XML/бейс64);
  - `extra_url` — дополнительный URL (используется в некоторых сценариях, например при работе с m3u8/mpd).
- Ключевая логика:

```ts
const file_hash = Bun.hash.wyhash(file, config.converters.seed).toString(16);
const convert = await convertFacade.get({ direction, file_hash });

if (["success", "failed"].includes(String(convert?.status))) {
  // сразу возвращаем имеющийся результат/ошибку + removeOn (когда будет удалён файл)
}

if (!convert) {
  const availableSpace = await checkAvailableSpace();
  if (!availableSpace.isOk) {
    return { status: "failed", message: "There isn't enough space..." };
  }

  await converterQueue.add(
    `converter (${direction} ${file_hash} ${extra_url})`,
    { direction, file, file_hash, extra_url },
    { removeOnComplete: { age: 3600, count: 1000 }, removeOnFail: true, debounce: { id: `${direction}-${file}`, ttl: 5000 } },
  );
}

return { status: "waiting", message: convert?.message ?? "We are converting the file, wait a bit" };
```

- Поведение:
  - если задача с таким `direction + file_hash` уже выполнена — возвращается кэшированный результат (успех или ошибка) с URL файла и временем удаления;
  - если задачи нет — проверяется дисковое пространство, затем создаётся job `converter` в Redis/BullMQ;
  - всегда возвращает либо `success/failed` (для готовых задач), либо `waiting`.

---

## 3. Queue/Worker и очистка

### 3.1. Worker и очереди (`src/worker.ts`)

```ts
const opts: QueueBaseOptions = {
  connection: { host, port, username, password },
  prefix: "mconv",
};

export const cleanerQueue = new Queue("cleaner", opts);
export const cleanerWorker = new Worker("cleaner", CleanerJob.processor, {...})
  .on("completed", CleanerJob.onCompleted)
  .on("failed", CleanerJob.onFailed);

export const converterQueue = new Queue("converter", opts);
export const converterWorker = new Worker("converter", ConvertJob.processor, {...})
  .on("completed", ConvertJob.onCompleted)
  .on("failed", ConvertJob.onFailed);

export async function initCleaner() {
  await cleanerQueue.upsertJobScheduler("clean-files", { every: 7_200_000 }); // 2 часа
}
```

- Используется `bullmq` с Redis в качестве backend-а для очередей.
- Две очереди:
  - `converter` (до 200 параллельных задач) — конвертация медиа;
  - `cleaner` (5 параллельных задач) — периодическая очистка файлов и/или базы.

### 3.2. Job конвертации (`src/jobs/convert.ts`)

```ts
export default abstract class ConverterJob {
  static MAX_TIME_TO_CONVERT = 300_000; // 5 минут

  static async processor(job: Job<ConvertJobOpts>) {
    const { direction, file, file_hash, extra_url } = job.data;
    const getBy = { direction, file_hash };

    await convertFacade.create({ ...getBy, status: "waiting" });

    const [fromFormat, toFormat] = direction.split("-") as [MediaFormat, MediaFormat];
    if (!/^http(s)?:\/\//.exec(file) && fromFormat !== "mpd") {
      throw new FailedConvertMedia();
    }

    let converter = BaseConverter;
    switch (direction) {
      case "m3u8-mp4": converter = M3U8Converter; break;
      case "m4a-mp4":
      case "m4v-mp4": converter = M4AVConverter; break;
      case "mpd-mp4": converter = MPDConverter; break;
    }

    const convertedFile = await asyncWithTimelimit(
      ConverterJob.MAX_TIME_TO_CONVERT,
      new converter(file, toFormat, extra_url).convert(),
      null,
    );
    if (!convertedFile || !(await convertedFile.exists())) {
      throw new FailedConvertMedia();
    }

    await convertFacade.update(getBy, {
      status: "success",
      message: "",
      download_url: getPublicFilePath(convertedFile),
    });
  }

  static async onFailed(job: Job<ConvertJobOpts> | undefined, error: Error) { ... }
  static onCompleted(job: Job) { ... }
}
```

- Логика:
  - создаёт запись в БД (`convertFacade.create`) со статусом `waiting`;
  - выбирает нужный класс-конвертер по `direction`;
  - запускает его с тайм-аутом 5 минут (`asyncWithTimelimit`);
  - при успехе — обновляет запись: `status=success`, `download_url` = публичный путь к файлу;
  - при ошибке — `onFailed` пишет в лог и ставит `status=failed`, `message` = текст ошибки.

### 3.3. Job очистки (`src/jobs/cleaner.ts`)

- По аналогии, использует `convertFacade` и файловую систему для удаления старых результатов и освобождения места (код не приведён здесь целиком, но назначение понятно из имени и README).

---

## 4. Конвертеры и внешние запросы

### 4.1. Сетевой слой (`src/libs/network.ts`)

```ts
export async function fetchWithTimeout(
  url: string | URL | Request,
  options: Record<string, any> = {
    headers: {
      "User-Agent": config.converters.userAgent,
    },
  },
) {
  const { timeout = 3000 } = options;
  const controller = new AbortController();
  const id = setTimeout(() => controller.abort(), timeout as number);

  const response = await fetch(url, { ...options, signal: controller.signal });
  clearTimeout(id);
  return response;
}
```

**Куда:**

- Любые URL, указанные в поле `file` (или в манифестах m3u8/mpd), то есть CDN-ы/сервера, где хостится исходное видео/аудио.

**Как:**

- Используется глобальный `fetch` Bun-а с:
  - заголовком `User-Agent` из `config.converters.userAgent`;
  - тайм-аутом (по умолчанию 3 секунды, может быть перегружен);
  - поддержкой Range-запросов (по заголовку `Range` в `options.headers`).

**Зачем:**

- Скачивать манифесты m3u8/mpd и сегменты `.ts`/`.m4s`/`.m4a`/`.m4v` напрямую с оригинального хоста, не используя внешние CLI-загрузчики (как `yt-dlp`).

### 4.2. Работа с файлами и диском (`src/libs/file.ts`)

```ts
function clearFileName(filename: string, filetype = ""): string { ... }
function getFileNameByUrl(fileUrl: string): string { ... }
function appendToFileName(filename: string, text: string): string { ... }
async function checkAvailableSpace() {
  const space = await checkDiskSpace(config.app.publicPath);
  const freeMegabytes = byteToMegaByte(space.free);
  return { freeMegabytes, isOk: freeMegabytes >= config.converters.minAvailableMegabytes };
}
```

- `checkAvailableSpace()` использует библиотеку `check-disk-space` для получения свободного места в разделе, где расположен `config.app.publicPath`.
- Это позволяет **до постановки задач в очередь** блокировать новые конверсии, если свободного места меньше порогового значения (`config.converters.minAvailableMegabytes`).

### 4.3. Базовый конвертер (`src/libs/converters/base.ts`)

```ts
export default class BaseConverter {
  blackImg = path.join(config.app.publicPath, "black.png");
  mediaMeta = path.join(config.app.publicPath, "meta.xml");

  tempPath: string;  // временная директория сегментов
  outPath: string;   // директория результата: public/media/<format>/<date>
  outputFilePath: string; // полный путь к итоговому файлу

  constructor(url: string, format: MediaFormat = "mp4", extraUrl: string | null = null) {
    this.url = url;
    this.extraUrl = extraUrl ?? url;
    this.format = format;
    const fileUUID = getUid();
    this.filename = `${fileUUID}.${format}`;
    const currentDate = getCurrentDate(true);
    this.tempPath = path.join(defaultTempPath, currentDate, fileUUID);
    this.outPath = path.join(config.app.publicPath, "media", format, currentDate);
    this.outputFilePath = path.join(this.outPath, clearFileName(this.filename, `.${format}`));
  }

  async createOutDir() { ... }
  async afterConvertCb() { ... } // чистит temp и возвращает Bun.file(outPath/filename)

  async convertToMP4(): Promise<any> { throw new Error("Not implemented"); }

  async convert(): Promise<any> {
    switch (this.format) {
      case "mp4": return await this.convertToMP4();
      default: throw new Error("Not implemented");
    }
  }
}
```

- Все конкретные конвертеры наследуются от `BaseConverter` и реализуют `convertToMP4()`.

### 4.4. Конвертация m3u8 → mp4 (`src/libs/converters/m3u8.ts`)

#### Загрузка манифеста и выбор лучшего потока

- Используется `m3u8-parser.Parser`.

```ts
async loadManifest(content: string) { ... } // если content — URL, скачивает его через downloadManifest

async getManifestWithBestBandwidth(url: string): Promise<Manifest> {
  let parsedManifest = await this.loadManifest(url);
  if (!parsedManifest.playlists?.length && !parsedManifest.mediaGroups?.AUDIO) {
    return parsedManifest;
  }

  this.hasOnlyAudio = !!(parsedManifest.mediaGroups?.AUDIO && Object.keys(...).length > 0);

  const bestUrl = this.hasOnlyAudio
    ? this.getMediaGroup(parsedManifest.mediaGroups!)
    : this.getBestPlaylist(parsedManifest.playlists!);

  url = this.replaceURLFileName(url, bestUrl.uri);
  this.url = url;
  parsedManifest = await this.loadManifest(url);
  return parsedManifest;
}
```

- Для HLS с отдельной аудиодорожкой (`hasOnlyAudio=true`) выбирает соответствующую AUDIO-группу, в противном случае — поток с максимальной `BANDWIDTH`.

#### Скачивание сегментов и сборка

- Для каждого сегмента `.ts`/init-сегмента:

```ts
segment.content = await this.downloadSegment(segmentUrl, segment, isPartial);
segment.filePath = path.join(this.tempPath, filename);
await Bun.write(segment.filePath, segment.content);
```

- Затем либо:
  - конкатенация через FFmpeg (`mergeSegments`, создаёт `sls.txt` и запускает `ffmpeg -f concat -i sls.txt -c copy out.mp4`);
  - или конкатенация через потоковое слияние файлов с учётом `map` (для CMAF/фрагментированных контейнеров).

**Внешние процессы:** `ffmpeg` CLI.

### 4.5. Конвертация m4a/m4v → mp4 (`src/libs/converters/m4av.ts`)

#### Скачивание файла

```ts
async fetchM4av(url: string): Promise<[Blob, string]> {
  const res = await fetchWithTimeout(url, { headers: { "User-Agent": config.converters.userAgent } });
  const file = await res.blob();
  const filename = getFileNameByUrl(url);
  const filePath = path.join(this.tempPath, filename);
  await Bun.write(filePath, file);
  return [file, filePath];
}
```

#### Конвертация через MP4Box

```ts
async convertToMP4Impl(filePath: string) {
  const hasOnlyAudio = !!/\.m4a/.exec(filePath);
  const proc = Bun.spawn([
    "mp4box",
    "-add", filePath,
    ...(hasOnlyAudio ? ["-add", this.blackImg] : []),
    "-patch", this.mediaMeta,
    "-quiet",
    "-new", this.outputFilePath,
  ], { onExit: (...) => this.onExit(..., "MP4Box") });
  await proc.exited; proc.kill();
}
```

- При `m4a` добавляется чёрное изображение `black.png` как видеодорожка, чтобы итоговый файл ответствовал ожиданиям `ffmpeg`.
- `meta.xml` патчит метаданные, чтобы избежать ошибок декодера.

### 4.6. Конвертация mpd → mp4 (`src/libs/converters/mpd.ts`)

- Наследуется от `M3U8Converter`, но использует `mpd-parser.parse` и другие типы (`Playlist`, `Segment`, `MediaGroup` из `@/types/mpd`).
- Умеет:
  - загружать MPD по URL или декодировать base64/строку XML;
  - выбирать лучший поток/аудиогруппу;
  - если `resolvedUri` манифеста указывает на `.m4a`/`.m4v`, передаёт URL в `M4AVConverter` (в этом случае MP4Box используется вместо ffmpeg);
  - иначе — скачивает сегменты `.m4s` и собирает их по аналогии с HLS (через concat и `concatSegmentsByMap`).

**Внешние запросы:**

- `fetchWithTimeout` для MPD и `.m4s` сегментов.

---

## 5. Хранилища и кеш

### 5.1. Redis-кеш (`src/cache/cache.ts`)

```ts
import { Redis } from "ioredis";

const { host, port, username, password } = config.redis;
export const cache = new Redis({ host, port, username, password });
```

- Сейчас используется как общий `Redis`-клиент (очевидная роль — кеш или вспомогательные операции для очередей и state, детали в `src/cache/repositories/*`).
- BullMQ также использует Redis для хранения состояния очередей/задач (через `QueueBaseOptions.connection`).

### 5.2. PostgreSQL

- Через `kysely` (см. `src/database/*`) хранятся данные о задачах конвертации (таблица с полями типа: `id`, `direction`, `file_hash`, `status`, `download_url`, `message`, `created_at` и т.д.).
- Это позволяет:
  - быстро находить уже выполненные/упавшие конверсии;
  - возвращать пользователю URL и время удаления файла.

---

## 6. Сводка по сетевому поведению

1. **Исходящие HTTP-запросы:**
   - `fetchWithTimeout` ходит по URL, переданным пользователем (`file`, а также вложенным ссылкам из манифестов m3u8/mpd), чтобы скачать:
     - m3u8/mpd манифесты;
     - сегменты `.ts`/`.m4s`;
     - файлы `.m4a`/`.m4v`.
   - Заголовок `User-Agent` берётся из `config.converters.userAgent`; также возможны Range-запросы через `headers.Range`.
2. **Внешние CLI-процессы:**
   - `ffmpeg` — объединение сегментов в итоговый файл (HLS/CMAF-потоки);
   - `mp4box` — сборка `.m4a`/`.m4v` в корректный `.mp4` (с патчем метаданных и, при необходимости, с добавлением видео из `black.png`).
   - Эти процессы запускаются через `Bun.spawn`, их статус логируется.
3. **Внутренние сетевые компоненты:**
   - Redis (`ioredis` + BullMQ) для очередей и, возможно, кеша;
   - PostgreSQL (`pg` + `kysely`) для хранения состояния задач;
   - Loki (опционально) через `@vaylo/pino` для логов (аналогично другим проектам FOSWLY).
4. **Сервис не обращается к Яндекс/VOT/YouTube API напрямую** — он работает только с уже доступными URL и протоколами HLS/DASH и контейнерами m4a/m4v, решая задачу: «по ссылке на поток/файл сконвертировать в `.mp4` и отдать стабильную ссылку на результат».
