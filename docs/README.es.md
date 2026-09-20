# procreepy

Una pequeña utilidad Unix multiplataforma (Linux, Windows, macOS): extrae el
timelapse de archivo ya preparado de un `.procreate` y une sus segmentos en
un solo MP4. No re-codifica nada (stream copy) y no renderiza nada.

`.procreate` es un ZIP. Si la grabación del timelapse estaba activada, dentro
hay

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

La utilidad toma exactamente estos archivos: los ordena **numéricamente**
(`segment-9` antes que `segment-10`), analiza la estructura MP4 de cada
segmento y los reconstruye en un MP4 moov-first copiando los fotogramas tal
cual. No abre `Document.archive`, las capas ni los fragmentos raster
(`*.lz4`).

## Requisitos

Sin dependencias externas: ni `ffmpeg` ni `ffprobe` son
necesarios. Para compilar solo se necesita Go (la versión está en `go.mod`).

```bash
go build -o procreepy ./cmd/procreepy
```

### Compilar para otros sistemas operativos

El proyecto es Go puro y se compila de forma cruzada sin problemas para
todos los destinos admitidos. Desde cualquier plataforma, cualquiera de
estos comandos funciona:

| Destino | Comando |
|---|---|
| Linux x86-64 | `GOOS=linux GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM de 64 bits (Raspberry Pi, Graviton) | `GOOS=linux GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM de 32 bits | `GOOS=linux GOARCH=arm GOARM=7 go build -o procreepy ./cmd/procreepy` |
| Windows x86-64 (10/11) | `GOOS=windows GOARCH=amd64 go build -o procreepy.exe ./cmd/procreepy` |
| Windows ARM de 64 bits | `GOOS=windows GOARCH=arm64 go build -o procreepy.exe ./cmd/procreepy` |
| macOS Intel | `GOOS=darwin GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| macOS Apple Silicon (M1–M5) | `GOOS=darwin GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |

Todas las compilaciones son estáticas (sin cgo): un binario de Linux se
ejecuta en cualquier distribución, sea cual sea su versión de glibc. La
pipeline de GitLab CI compila exactamente estos destinos en cada commit; el
trabajo `dist` publica los tarballs junto con un manifiesto `SHA256SUMS`, y
los trabajos `repro:*` demuestran que los binarios son reproducibles bit a
bit.

- Linux/macOS: no hay paso de instalación, ejecute el binario directamente.
- Windows: el binario no está firmado, así que SmartScreen puede mostrar
  «Se protegió tu equipo» — elija **Más información → Ejecutar de todos
  modos**.

## Uso

Un archivo:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```

Se soportan las cuatro combinaciones `INPUT`/`OUTPUT`: `FILE OUTPUT`,
`FILE -`, `- OUTPUT`, `- -`. Si `OUTPUT` se omite, es stdout. Si `OUTPUT` es
un directorio existente, el vídeo se coloca ahí con el nombre original
(`procreepy art.procreate videos/` → `videos/art.mp4`).

### Modo por lotes: una carpeta de `.procreate` → una carpeta de vídeos

El escenario "tengo `input/` lleno de `.procreate` y quiero los vídeos en
`output/timelaps/`":

```bash
procreepy input/
```

```text
input/                              output/timelaps/
├── Portrait of a Cat.procreate →   ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →   ├── Landscape v2.mp4
└── No Timelapse.procreate              └── (omitido, con aviso)
```

- **Nombres**: `<nombre original sin .procreate>.mp4`. Espacios, cirílico y
  caracteres especiales se conservan tal cual.
- **Carpeta de salida**: por defecto `output/timelaps/` relativa al
  directorio actual, se crea sola. Otra puede indicarse como segundo
  argumento: `procreepy input/ ~/Videos/procreate`.
- **Repetir la ejecución es seguro**: los vídeos que ya existen se omiten.
  Para reconstruir todo: `--force` (`-f`).
- **`-r`** también entra en subcarpetas; la estructura de subcarpetas se
  refleja en el resultado (`input/2025/Cat.procreate` →
  `output/timelaps/2025/Cat.mp4`), así que nombres iguales en carpetas
  distintas no colisionan.
- **Un archivo malo no detiene a los demás.** Un archivo sin timelapse (la
  grabación estaba desactivada) es un aviso, no un error. Un archivo dañado es
  un error: aparecerá en el resumen final y el código de salida será `1`.
- Los archivos ocultos (`._Foo.procreate`, que macOS deja al copiar) se
  ignoran.
- Los originales nunca se modifican.

Salida de ejemplo (todo va a stderr):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`--list` y `--verify` también aceptan un directorio y recorren todos los
archivos.

### Dividir: vídeo + proyecto aligerado (`--split`)

La idea: los timelapses ocupan más espacio que el propio dibujo — por
ejemplo, al hacer copia de seguridad en un iPad puede ser útil mantenerlos
separados. `--split` escribe, junto a cada `MP4` terminado, una copia
aligerada del proyecto **sin** nada bajo `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (el timelapse, lossless)
                     →  artwork.procreepy.procreate  (el mismo proyecto, sin video/)
```

En modo por lotes, igual: junto a cada `X.mp4` aparece un
`X.procreepy.procreate`. Todos los demás miembros del archivo (capas,
`Info.plist`, vistas previas) se trasladan byte a byte: el orden, los métodos
de compresión y las marcas de tiempo se conservan. El `.procreate` original
no se modifica; la copia aligerada no puede escribirse a stdout, así que
`--split` requiere un `OUTPUT` de archivo.

### Diagnóstico

```bash
procreepy --list artwork.procreate
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
procreepy --verify artwork.procreate
```

Analiza cada segmento directamente del archivo (incluida la verificación de
CRC dentro del ZIP), imprime un informe línea a línea y comprueba que los
segmentos pueden unirse sin re-codificar. **No se crea ningún vídeo de
salida.** `--list` solo lee el directorio del ZIP.

## Opciones

| Opción | Qué hace |
|---|---|
| `-r`, `--recursive` | entrada directorio: recorrer también subcarpetas |
| `-f`, `--force` | entrada directorio: sobrescribir vídeos que ya existen |
| `--strict` | tratar números de segmento ausentes como error (por defecto es un aviso) |
| `--reencode` | aceptada por compatibilidad con guiones antiguos; no hay re-codificación, siempre es stream copy |
| `--split` | escribir `X.procreepy.procreate` junto a cada `MP4` — el proyecto sin `video/` |
| `--tmpdir DIR` | dónde descomprimir los segmentos |
| `-q`, `--quiet` | imprimir solo avisos y errores |

## Cómo funciona

1. `INPUT` es un archivo o stdin. Stdin (y cualquier entrada no
   posicionable) se vuelca primero a un archivo temporal, porque ZIP exige
   acceso aleatorio.
2. Se valida el ZIP (solo lectura) y se localizan las entradas
   `video/segments/segment-N.mp4`.
3. Ordenación numérica. Los huecos en la numeración son un aviso; los nombres
   sin número se ignoran con un aviso.
4. Cada segmento se analiza directamente del ZIP (sin extracción completa):
   cajas MP4, tamaños de pista, parámetros del códec. Ante el primer daño —
   parada.
5. Verificación de compatibilidad (resolución, códec, conjuntos SPS/PPS,
   audio). De lo contrario `-c copy` produciría basura silenciosamente — por
   eso la incompatibilidad es un error con un mensaje claro, no una sorpresa
   en el vídeo final.
6. Se ensambla el MP4 moov-first: `ftyp`, `moov` (todas las pistas, cortadas
   de los segmentos), luego `mdat` tras `mdat` en orden de reproducción.
7. El archivo temporal (si lo hay) se elimina siempre — en caso de éxito, de
   error, con Ctrl+C y con SIGTERM.

### Escritura a archivo y a stdout

Ambos caminos ensamblan el mismo MP4 moov-first: el átomo moov se escribe
primero, porque los fotogramas se copian directamente de los segmentos de
origen y los metadatos se conocen antes de empezar a escribir. Para un
archivo es un MP4 "clásico", apto para reproductores y editores por igual; el
mismo archivo exacto va a la tubería — `> artwork.mp4` produce el mismo
resultado que un `procreepy artwork.procreate artwork.mp4` explícito.

- La salida a **archivo** es atómica: un `.partial` junto al destino y
  renombrado solo tras el éxito. Una ejecución fallida no deja restos ni
  corrompe un archivo existente.
- stdout nunca se ensucia con texto. Todos los registros
  `level=INFO`/`WARN`/`ERROR` (un registro estructurado key=value por línea)
  van a stderr. La única excepción es el informe
  `--list`/`--verify`, donde stdout *es* el resultado. Si stdout es un
  terminal, la utilidad se niega a volcar un MP4 binario en él.

### Archivos temporales y Fedora

En Fedora, `/tmp` es un tmpfs en RAM. Los segmentos del timelapse pueden
pesar cientos de megabytes, y al leer desde stdin se vuelca el `.procreate`
entero. Por eso el directorio temporal se elige así: `--tmpdir` → `$TMPDIR`
→ `/var/tmp` (en disco). Antes de descomprimir se comprueba el espacio
libre; si no alcanza, se obtiene un error claro con una pista en vez de un
"No space left" a mitad de la tarea.

## Códigos de salida

| Código | Significado |
|---|---|
| 0 | éxito |
| 1 | error inesperado; en modo por lotes — al menos un archivo falló |
| 2 | argumentos incorrectos; la salida sobreescribiría la entrada; stdout es un terminal |
| 3 | entrada no encontrada, vacía o no es un ZIP |
| 4 | no hay `video/segments` en el archivo (no se grabó timelapse) |
| 5 | segmento dañado; numeración ambigua o ausente (`--strict`) |
| 6 | reservado (en desuso: no hay dependencias externas) |
| 7 | segmentos incompatibles para stream copy |
| 8 | reservado (en desuso: no hay dependencias externas) |
| 9 | fallo al escribir el resultado o los archivos temporales |
| 130 | interrumpido (Ctrl+C / SIGTERM) |

## Pruebas

```bash
go test ./...
```

No se necesitan `.procreate` reales: las pruebas construyen ZIPs a partir de
segmentos MP4 generados (ver `internal/testkit`). Las comprobaciones son
estructurales: análisis del MP4 resultante, orden de cajas, número de
muestras, contenido de `mdat`. `-race` no es necesario, pero funciona si hay
un compilador C instalado.

Cubierto: el archivo ordinario, la ausencia de `video/segments`, un solo
segmento, segmentos fuera de orden (`segment-9`/`segment-10`), stdin, stdout,
espacios y caracteres especiales en los nombres, ZIPs dañados y truncados,
MP4s dañados y truncados, daños de CRC, errores de escritura (`/dev/full`),
segmentos incompatibles y todo el modo por lotes.

## Lo que la utilidad deliberadamente no hace

No analiza `Document.archive` (NSKeyedArchive), no toca `*.lz4`, no restaura
capas y no renderiza la imagen. Si un timelapse no fue grabado en el archivo,
esta utilidad no puede recuperarlo del historial de dibujo. Nota: `lz4 -t`
sobre un `.lz4` sacado de un `.procreate` no es una verificación de
integridad — no son fotogramas LZ4 independientes.

## Referencias del formato

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Licencia

Apache License 2.0, ver `LICENSE`.

---

## Idiomas

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
