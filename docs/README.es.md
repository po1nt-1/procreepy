# procreepy

Una pequeña utilidad multiplataforma (Linux, Windows, macOS): extrae el
timelapse de archivo ya preparado de un `.procreate` y une sus segmentos en
un solo MP4. No recodifica nada (stream copy) ni renderiza nada.

`.procreate` es un archivo ZIP. Si la grabación del timelapse estaba activada,
contiene

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
La utilidad toma exactamente estos archivos: los ordena **numéricamente**
(`segment-9` antes que `segment-10`), analiza la estructura MP4 de cada segmento
y los reconstruye en un único MP4 moov-first copiando los fotogramas tal cual.
Nunca abre `Document.archive`, las capas ni los fragmentos raster (`*.lz4`).

## Requisitos

No hay dependencias externas: no se necesitan `ffmpeg` ni `ffprobe`. Para compilar solo se necesitan Go (la versión está en `go.mod`) y `make` (presente en todas las plataformas compatibles; en sistemas mínimos se instala con el gestor de paquetes).
El Makefile es la entrada canónica para la compilación: fija el mismo entorno hermético que usa CI (modo offline para módulos, toolchain local, sin cgo) y localiza automáticamente la toolchain de Go.

```bash
make build      # compilar todo y generar un ./procreepy ejecutable
make check      # gofmt + compilación + vet + suite completa de pruebas
```

`make build` marca el binario con `dev-<commit>`, de modo que `procreepy --version` indica de qué commit procede (los tarballs de release llevan la etiqueta en su lugar). Sin `make`, el equivalente directo es `go build ./... && go build -o procreepy ./cmd/procreepy` (ese binario informa `dev`, o `dev-<commit>` cuando se compila dentro de un checkout de Git).

### Compilar para otros sistemas operativos

El proyecto está escrito en Go puro y se puede compilar de forma cruzada para todos los destinos admitidos. Desde cualquier plataforma:

| Destino | Comando |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM de 64 bits (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM de 32 bits | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM de 64 bits | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |
Cada destino genera un `dist/procreepy-<version>-<os>-<arch>.tar.gz` normalizado y muestra su SHA-256; `make cross` compila toda la matriz de una vez y `make repro` demuestra que una compilación es reproducible bit a bit. El equivalente directo para un destino es `GOOS=… GOARCH=… go build -o procreepy[.exe] ./cmd/procreepy`.
Todas las compilaciones son estáticas (sin cgo): un binario de Linux se ejecuta en cualquier distribución, independientemente de su versión de glibc. Las canalizaciones de CI (GitLab y GitHub) compilan exactamente estos destinos en cada commit; el trabajo `dist` publica los tarballs junto con un manifiesto `SHA256SUMS`, y los trabajos `repro:*` demuestran la reproducibilidad bit a bit de los binarios.

- Linux/macOS: no hay paso de instalación; ejecuta el binario directamente.
- Windows: el binario no está firmado, por lo que SmartScreen puede mostrar «Protegió su PC» — elige **Más información → Ejecutar de todos modos**.

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
Se admiten las cuatro combinaciones `INPUT`/`OUTPUT`:
`FILE OUTPUT`, `FILE -`, `- OUTPUT`, `- -`. Si se omite `OUTPUT`, se usa stdout.
Si `OUTPUT` es un directorio existente, el vídeo se coloca allí con el nombre
original (`procreepy art.procreate videos/` → `videos/art.mp4`).

### Modo por lotes: una carpeta de `.procreate` → una carpeta de vídeos

El escenario «tengo `input/` lleno de archivos `.procreate` y quiero los vídeos
en `output/timelaps/`»:

```bash
procreepy input/
```

```text
input/                              output/timelaps/
├── Portrait of a Cat.procreate →   ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →   ├── Landscape v2.mp4
└── No Timelapse.procreate              └── (omitido, con una advertencia)
```
- **Nombres**: `<nombre original sin .procreate>.mp4`. Los espacios, el cirílico
y los caracteres especiales se conservan tal cual.
- **Carpeta de salida**: por defecto `output/timelaps/`, relativa al directorio
actual, se crea automáticamente. Se puede indicar otra como segundo argumento:
`procreepy input/ ~/Videos/procreate`.
- **Volver a ejecutar es seguro**: los vídeos que ya existen se omiten. Para
reconstruirlo todo: `--force` (`-f`).
- **`-r`** también desciende a las subcarpetas; su estructura se replica en el
resultado (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`), por lo
que los nombres idénticos en carpetas distintas no entran en conflicto.
- **Un archivo defectuoso no detiene los demás.** Un archivo sin timelapse (la
grabación estaba desactivada) es una advertencia, no un error. Un archivo
corrupto es un error: aparece en el resumen final y el código de salida pasa a
ser `1`.
- Los archivos ocultos (`._Foo.procreate`, que macOS deja al copiar) se ignoran.
- Los originales nunca se modifican.
Ejemplo de salida (todo va a stderr):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```
`--list` y `--verify` también aceptan un directorio y recorren todos sus archivos.

### Dividir: vídeo + proyecto aligerado (`--split`)

La idea: los timelapses ocupan más espacio que el propio dibujo — por ejemplo,
al hacer una copia de seguridad en un iPad puede ser útil mantenerlos separados.
`--split` escribe, junto a cada `MP4` terminado, una copia aligerada del
proyecto **sin** nada bajo `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (el timelapse, lossless)
                     →  artwork.procreepy.procreate  (el mismo proyecto, sin video/)
```
En modo por lotes sucede lo mismo: junto a cada `X.mp4` aparece un
`X.procreepy.procreate`. Todos los demás miembros del archivo (capas,
`Info.plist`, vistas previas) se conservan byte a byte: orden, métodos de
compresión y marcas de tiempo. El `.procreate` original no se modifica; la
copia aligerada no puede escribirse en stdout, por lo que `--split` requiere
un `OUTPUT` de archivo.

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
Analiza cada segmento directamente desde el archivo (incluida la comprobación
de CRC dentro del ZIP), muestra un informe línea por línea y comprueba que los
segmentos se pueden unir sin recodificar. **No se crea ningún vídeo de salida.**
`--list` solo lee el directorio del ZIP.

## Opciones

| Opción | Qué hace |
|---|---|
| `-r`, `--recursive` | con una carpeta de entrada: recorrer también las subcarpetas |
| `-f`, `--force` | con una carpeta de entrada: sobrescribir los vídeos que ya existen |
| `--strict` | tratar los números de segmento que faltan como un error (por defecto, advertencia) |
| `--reencode` | se acepta por compatibilidad con scripts antiguos; no hay recodificación, siempre es stream copy |
| `--split` | escribir `X.procreepy.procreate` junto a cada `MP4` — el proyecto sin `video/` |
| `--tmpdir DIR` | dónde colocar los archivos temporales |
| `-q`, `--quiet` | mostrar solo advertencias y errores |

## Cómo funciona

1. `INPUT` es un archivo o stdin. Stdin (y cualquier entrada no buscable) se
   vuelca primero a un archivo temporal, porque ZIP requiere acceso aleatorio.
2. Se valida el ZIP (solo lectura) y se localizan las entradas
   `video/segments/segment-N.mp4`.
3. Ordenación numérica. Los huecos en la numeración son una advertencia; los
   nombres sin número se ignoran con una advertencia.
4. Cada segmento se analiza directamente desde el ZIP (sin extraerlo por completo):
   cajas MP4, tamaños de pista y parámetros del códec. Ante la primera
   corrupción, se detiene el proceso.
5. Comprobación de compatibilidad (resolución, códec, conjuntos SPS/PPS, audio).
   De lo contrario, `-c copy` produciría basura silenciosamente; por eso una
   incompatibilidad es un error con un mensaje claro, no una sorpresa en el
   vídeo terminado.
6. Se ensambla el MP4 moov-first: `ftyp`, `moov` (todas las pistas, recortadas
   de los segmentos) y después `mdat` tras `mdat`, en orden de reproducción.
7. El archivo temporal, si existe, se elimina siempre — tras un éxito, un error,
   Ctrl+C o SIGTERM.

### Escritura a un archivo y a stdout

Ambas vías ensamblan el mismo MP4 moov-first: el átomo moov se escribe primero,
porque los fotogramas se copian directamente de los segmentos de origen y los
metadatos ya se conocen antes de empezar a escribir. Para un archivo es un MP4
«clásico», adecuado tanto para reproductores como para editores; exactamente el
mismo archivo se envía a una tubería — `> artwork.mp4` produce el mismo resultado
que un `procreepy artwork.procreate artwork.mp4` explícito.
  * **Salida a archivo** es atómica: se crea un `.partial` junto al destino y se
    renombra solo tras el éxito. Una ejecución fallida no deja restos ni corrompe
    un archivo existente.
  * stdout nunca se contamina con texto. Todas las líneas de registro
    (`level=INFO`/`WARN`/`ERROR`, un registro estructurado key=value por línea)
    van a stderr. La única excepción es el informe `--list`/`--verify`, donde
    stdout es el resultado. Si stdout es un terminal, la utilidad se niega a
    volcar en él un MP4 binario.

### Archivos temporales y Fedora

En Fedora, `/tmp` es un tmpfs en RAM. Los segmentos de timelapse pueden ocupar
cientos de megabytes y, al leer desde stdin, se vuelca todo el `.procreate`.
Por eso, el directorio temporal se elige así: `--tmpdir` → `$TMPDIR` →
`/var/tmp` (en disco) → el directorio predeterminado del sistema. Si el disco
se llena mientras se vuelca el archivo, se obtiene un error claro con una
indicación (usar `--tmpdir` en un directorio respaldado por disco más grande)
en lugar de un simple «No space left».

## Códigos de salida

| Código | Significado |
|---|---|
| 0 | éxito |
| 1 | error inesperado; en modo por lotes — al menos un archivo falló |
| 2 | argumentos incorrectos; la salida sobrescribiría la entrada; stdout es un terminal |
| 3 | entrada no encontrada, vacía o no es un ZIP |
| 4 | no hay `video/segments` en el archivo (no se grabó ningún timelapse) |
| 5 | segmento corrupto; numeración ambigua o ausente (`--strict`) |
| 6 | reservado (no se usa: no hay dependencias externas) |
| 7 | los segmentos son incompatibles para stream copy |
| 8 | reservado (no se usa: no hay dependencias externas) |
| 9 | no se pudo escribir el resultado o los archivos temporales |
| 130 | interrumpido (Ctrl+C / SIGTERM) |

## Pruebas

```bash
make test          # or: go test ./...
```
No se necesitan archivos `.procreate` reales: las pruebas construyen ZIP a
partir de segmentos MP4 generados (véase `internal/testkit`). Las comprobaciones
son estructurales: análisis del MP4 resultante, orden de las cajas, número de
muestras y contenido de `mdat`. `-race` no es necesario, pero funciona si hay
un compilador de C instalado.
Cubierto: archivo normal, ausencia de `video/segments`, un solo segmento,
segmentos fuera de orden (`segment-9`/`segment-10`), stdin, stdout, espacios y
caracteres especiales en los nombres, ZIP corruptos y truncados, MP4 corruptos
y truncados, daños de CRC, errores de escritura (`/dev/full`), segmentos
incompatibles y todo el modo por lotes.

## Lo que la utilidad deliberadamente no hace

No analiza `Document.archive` (NSKeyedArchive), no toca `*.lz4`, no restaura
capas ni renderiza la imagen. Si el timelapse no se grabó en el archivo, esta
utilidad no puede recuperarlo del historial de dibujo. Nota: `lz4 -t` sobre un
`.lz4` extraído de un `.procreate` no es una comprobación de integridad — no son
fotogramas LZ4 independientes.

## Referencias del formato

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Licencia

Apache License 2.0, véase `LICENSE`.

---

## Idiomas

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
