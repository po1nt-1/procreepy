# procreate-video

Маленькая Unix-утилита для Linux/Fedora: достаёт из `.procreate` уже готовый
архивный timelapse и собирает из его сегментов один MP4. Ничего не
перекодирует (stream copy), ничего не рендерит.

`.procreate` — это ZIP. Если запись таймлапса была включена, внутри лежит

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

Утилита берёт именно эти файлы: сортирует **численно** (`segment-9` раньше
`segment-10`), проверяет через `ffprobe` и склеивает FFmpeg concat demuxer'ом.
`Document.archive`, слои и raster-чанки (`*.lz4`) она не открывает вообще.

## Требования

- Python 3.9+ (только стандартная библиотека)
- `ffmpeg` и `ffprobe`

```bash
sudo dnf install ffmpeg-free      # или полный ffmpeg из RPM Fusion
```

Для склейки (`-c copy`) кодеры не нужны. Полный `ffmpeg` с `libx264` требуется
только для необязательного флага `--reencode`.

## Установка

Без установки — запускать прямо из репозитория:

```bash
./procreate-video artwork.procreate artwork.mp4
```

Или поставить команду в `PATH`:

```bash
pip install --user .          # даст команду procreate-video в ~/.local/bin
# либо просто симлинк:
ln -s "$PWD/procreate-video" ~/.local/bin/procreate-video
```

## Использование

Один файл:

```bash
procreate-video artwork.procreate artwork.mp4
procreate-video artwork.procreate > artwork.mp4
cat artwork.procreate | procreate-video - > artwork.mp4
procreate-video --list artwork.procreate
procreate-video --verify artwork.procreate
```

Поддерживаются все четыре комбинации `INPUT`/`OUTPUT`:
`FILE OUTPUT`, `FILE -`, `- OUTPUT`, `- -`. Если `OUTPUT` опущен, это stdout.
Если `OUTPUT` — существующая папка, видео кладётся в неё под именем оригинала
(`procreate-video art.procreate videos/` → `videos/art.mp4`).

### Пакетный режим: папка `.procreate` → папка с видео

Сценарий «есть `input/` с кучей `.procreate`, нужно сложить видео в
`output/timelaps/`»:

```bash
procreate-video input/
```

```text
input/                                output/timelaps/
├── Портрет кота.procreate      →     ├── Портрет кота.mp4
├── Landscape v2.procreate      →     ├── Landscape v2.mp4
└── Без таймлапса.procreate           └── (пропущен, предупреждение)
```

- **Имена**: `<имя оригинала без .procreate>.mp4`. Пробелы, кириллица и
  спецсимволы сохраняются как есть.
- **Папка результата**: по умолчанию `output/timelaps/` относительно текущей
  папки, создаётся сама. Другую можно указать вторым аргументом:
  `procreate-video input/ ~/Videos/procreate`.
- **Повторный запуск безопасен**: уже существующие видео пропускаются.
  Пересобрать всё заново: `--force` (`-f`).
- **`-r`** — заходить и в подпапки; структура подпапок повторяется в результате
  (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`), так что
  одинаковые имена в разных папках не конфликтуют.
- **Один плохой файл не останавливает остальные.** Файл без таймлапса
  (запись была выключена) — это предупреждение, а не ошибка. Битый файл — ошибка:
  он попадёт в итоговую сводку, а код выхода будет `1`.
- Скрытые файлы (`._Foo.procreate`, которые macOS оставляет при копировании)
  игнорируются.
- Оригиналы не изменяются.

Пример вывода (всё это идёт в stderr):

```text
info: 4 .procreate file(s) in input -> output/timelaps/
info: [1/4] input/Landscape v2.procreate -> output/timelaps/Landscape v2.mp4
warning: [2/4] input/Без таймлапса.procreate: no timelapse video inside, skipped
error: [3/4] input/Битый файл.procreate: input is not a valid ZIP archive ...
info: [4/4] input/Портрет кота.procreate -> output/timelaps/Портрет кота.mp4
info: summary: 2 converted, 1 without timelapse, 1 FAILED
```

`--list` и `--verify` тоже принимают папку и проходят по всем файлам.

### Диагностика

```bash
procreate-video --list artwork.procreate
```

```text
input: artwork.procreate
segments: 12

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
...
12 video/segments/segment-12.mp4
```

```bash
procreate-video --verify artwork.procreate
```

Извлекает каждый сегмент во временную папку, проверяет `ffprobe`'ом (и
CRC внутри ZIP), показывает построчный отчёт и проверяет, что сегменты можно
склеить без перекодирования. Итоговое видео **не создаётся**.
`--list` не требует ни `ffmpeg`, ни `ffprobe`.

## Опции

| Опция | Что делает |
|---|---|
| `-r`, `--recursive` | папка на входе: обходить и подпапки |
| `-f`, `--force` | папка на входе: перезаписывать уже готовые видео |
| `--strict` | пропущенные номера сегментов — ошибка (по умолчанию предупреждение) |
| `--reencode` | если сегменты нельзя склеить копированием, перекодировать в H.264 |
| `--tmpdir DIR` | куда распаковывать сегменты |
| `-q`, `--quiet` | печатать только warning/error |

## Как это работает

1. `INPUT` — файл или stdin. Stdin (и не-seekable вход вроде `<(cat x)`)
   сначала спулится во временный файл, потому что ZIP требует произвольного
   доступа.
2. Проверка ZIP (`zipfile`, только чтение), поиск `video/segments/segment-N.mp4`.
3. Численная сортировка. Пропуски в нумерации — предупреждение; имена без
   номера игнорируются с предупреждением.
4. Сегменты по одному извлекаются во временную папку и сразу проверяются
   `ffprobe`'ом — на первом же битом остановка, без распаковки остального.
5. Проверка совместимости (кодек, размер, pix_fmt, аудио). Иначе `-c copy`
   молча даст мусор — поэтому несовместимость это ошибка с понятным
   сообщением, а не сюрприз в готовом видео.
6. `ffmpeg -f concat -safe 0 -i concat.txt -c copy …`
7. Временная папка удаляется всегда — при успехе, ошибке, Ctrl+C и SIGTERM.

### Вывод в файл и в stdout — это разные MP4

- В **файл** пишется обычный MP4 (`+faststart`). Пишется во временный
  `.partial` рядом и переименовывается только после успеха: неудачный запуск
  не оставляет обрубков и не портит уже существующий файл.
- В **stdout** пишется fragmented MP4 (`+frag_keyframe+empty_moov`), потому
  что обычный MP4 требует seek, а pipe его не даёт. Он нормально играется в
  mpv/VLC/браузерах/ffmpeg, но некоторые редакторы предпочитают обычный MP4.
  Это касается и `> artwork.mp4`. Если нужен «классический» файл, используйте
  `procreate-video artwork.procreate artwork.mp4`.
- stdout никогда не пачкается текстом. Все `info:`/`warning:`/`error:` идут в
  stderr. Единственное исключение — отчёт `--list`/`--verify`, где stdout
  и есть результат. Если stdout — терминал, утилита отказывается
  сваливать туда двоичный MP4.

### Временные файлы и Fedora

На Fedora `/tmp` — это tmpfs в оперативной памяти. Сегменты таймлапса могут быть
сотни мегабайт, а при чтении из stdin спулится весь `.procreate`. Поэтому
временная папка выбирается так: `--tmpdir` → `$TMPDIR` → `/var/tmp` (на диске).
Перед распаковкой проверяется свободное место; если его не хватает, будет
понятная ошибка с подсказкой, а не «No space left» посреди работы.

## Коды выхода

| Код | Значение |
|---|---|
| 0 | успех |
| 1 | непредвиденная ошибка; в пакетном режиме — хотя бы один файл не удался |
| 2 | неверные аргументы; вывод перезаписал бы вход; stdout — терминал |
| 3 | вход не найден, пуст или не является ZIP |
| 4 | в архиве нет `video/segments` (таймлапс не записывался) |
| 5 | сегмент повреждён; двусмысленная или отсутствующая нумерация (`--strict`) |
| 6 | нет `ffmpeg`/`ffprobe` (или нет `libx264` для `--reencode`) |
| 7 | сегменты несовместимы для `-c copy` |
| 8 | `ffmpeg` завершился с ошибкой |
| 9 | ошибка записи результата или временных файлов |
| 130 | прервано (Ctrl+C / SIGTERM) |

## Тесты

```bash
sudo dnf install python3-pytest
python3 -m pytest
```

Настоящие `.procreate` не нужны: тесты собирают ZIP из сгенерированных
одноцветных MP4-сегментов (по цвету на сегмент). Поэтому порядок склейки
проверяется по-настоящему: итоговое видео декодируется, и сверяются цвета
кадров. Основные проверки идут через `ffprobe`/`ffmpeg` (число кадров,
длительность, полное декодирование), а не через `cmp`: два независимых
муксинга не обязаны давать одинаковые байты.

Покрыто: обычный файл, отсутствие `video/segments`, один сегмент, 12
сегментов в перемешанном порядке (`segment-9`/`segment-10`), stdin, stdout,
пробелы и спецсимволы в именах, битый и обрезанный ZIP, битый и обрезанный
MP4, порча CRC, отсутствие `ffmpeg`/`ffprobe`, ошибки записи (`/dev/full`),
несовместимые сегменты, прерывание сигналом, весь пакетный режим.

## Чего утилита намеренно не делает

Не разбирает `Document.archive` (NSKeyedArchive), не трогает `*.lz4`, не
восстанавливает слои и не рендерит изображение. Если таймлапс в файле не
записывался, восстановить его из истории рисования эта утилита не может.
Заметьте: `lz4 -t` на `.lz4` из `.procreate` не является проверкой целостности —
это не самостоятельные LZ4-фреймы.

## Источники по формату

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer
- FFmpeg concat demuxer — https://ffmpeg.org/ffmpeg-formats.html#concat

## Лицензия

MIT, см. `LICENSE`.
