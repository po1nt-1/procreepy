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
En modo por lotes también escribe, para cada obra convertida, una copia del
proyecto lista para volver a importarse sin el timelapse y — con `--psd` —
un PSD de Photoshop con capas. El camino del timelapse nunca abre
`Document.archive`, las capas ni los fragmentos raster (`*.lz4`); solo
`--psd` lo hace.

## Requisitos

No hay dependencias externas: no se necesitan `ffmpeg` ni `ffprobe`. Para compilar solo se necesitan Go (la versión está en `go.mod`) y `make` (presente en todas las plataformas compatibles; en sistemas mínimos se instala con el gestor de paquetes).
El Makefile es la entrada canónica para la compilación: fija el mismo entorno hermético que usa CI (modo offline para módulos, toolchain local, sin cgo) y localiza automáticamente la toolchain de Go.

```bash
make build      # compilar todo y generar un ./procreepy ejecutable
make check      # gofmt + compilación + vet + suite completa de pruebas
```

`make build` marca el binario con `dev-<short sha>` (fuera de un checkout de Git: `dev-nogit`), de modo que `procreepy --version` indica de dónde procede (los tarballs de release llevan la etiqueta en su lugar). Sin `make`, el equivalente directo es `go build ./... && go build -o procreepy ./cmd/procreepy` (ese binario informa `dev`, o `dev-<commit>` cuando el stamping de VCS está disponible).

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
Todas las compilaciones son estáticas (sin cgo): un binario de Linux se ejecuta en cualquier distribución, independientemente de su versión de glibc. Las canalizaciones de CI (GitLab y GitHub) compilan exactamente estos destinos en cada commit y, dado que Windows es la plataforma principal, ejecutan además la suite completa de pruebas contra el binario `windows/amd64` real: de forma nativa en el runner alojado de GitHub y bajo Wine en GitLab, que solo tiene runners de Linux (véase `ci/windows-wine/`); el trabajo `dist` publica los tarballs junto con un manifiesto `SHA256SUMS`, y los trabajos `repro:*` demuestran la reproducibilidad bit a bit de los binarios.

- Linux/macOS: no hay paso de instalación; ejecuta el binario directamente.
- Windows: el binario no está firmado, por lo que SmartScreen puede mostrar «Protegió su PC» — elige **Más información → Ejecutar de todos modos**.

## Ejecución en un contenedor

Cada etiqueta de versión publica además una imagen multiarquitectura (`linux/amd64`, `linux/arm64`) en el registro del proyecto, así que la CLI se ejecuta sin cadena de herramientas de Go y sin descomprimir ningún tarball. Esto cubre también Apple Silicon: Docker Desktop y `podman machine` levantan una máquina virtual `linux/arm64` en un Mac de la serie M, por lo que la variante arm64 se selecciona automáticamente, sin la opción `--platform` y sin emulación.

```bash
# un solo archivo: monte el directorio actual y use rutas dentro de él
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4

# modo por lotes: una carpeta de entrada, una carpeta de salida
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest input/ output/

# stdin → stdout no necesita ningún montaje
docker run --rm -i registry.gitlab.com/po1nt-1/procreepy:latest - \
  < artwork.procreate > artwork.mp4
```

`podman` sustituye a `docker` sin cambios. Para fijar una versión use `:1.2.3` en lugar de `:latest` (las etiquetas de imagen no llevan el prefijo `v`); `latest` nunca se mueve a una etiqueta de prelanzamiento.

- **Propiedad de los archivos.** La imagen se ejecuta con uid 65532 (`distroless/static:nonroot`). En un host Linux añada `--user "$(id -u):$(id -g)"` para que los archivos de salida le pertenezcan; Docker Desktop en macOS asigna la propiedad del montaje por sí mismo y no necesita nada.
- **Sin shell dentro.** La imagen contiene solo el binario estático, por lo que `docker run … --help` o `… --verify file.procreate` funcionan, pero no hay ninguna `sh` a la que entrar.
- **Los archivos temporales** van a la capa escribible del contenedor, no al montaje. Un timelapse grande puede necesitar espacio allí; `--tmpdir /data/tmp` los traslada al volumen montado.
- **Proyectos privados.** El acceso al registro sigue la visibilidad del proyecto: en un proyecto público el pull es anónimo; en caso contrario, ejecute primero `docker login registry.gitlab.com`.

## Uso

Un archivo:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
```
Se admiten las cuatro combinaciones `INPUT`/`OUTPUT`:
`FILE OUTPUT`, `FILE -`, `- OUTPUT`, `- -`. Si se omite `OUTPUT`, se usa stdout.
Si `OUTPUT` es un directorio existente, el vídeo se coloca allí con el nombre
original (`procreepy art.procreate videos/` → `videos/art.mp4`).

### Modo por lotes: una carpeta de `.procreate` → carpetas de vídeos y proyectos

El escenario «tengo `input/` lleno de archivos `.procreate` y quiero los
resultados en `output/`»:

```bash
procreepy input/
```

Cada obra convertida produce una pareja — el timelapse y un proyecto
reinportable sin él:

```text
input/                               output/
├── Portrait of a Cat.procreate  →   ├── timelapses/Portrait of a Cat.mp4
│                                    ├── projects/Portrait of a Cat.procreepy.procreate
├── Landscape v2.procreate       →   ├── timelapses/Landscape v2.mp4
│                                    └── projects/Landscape v2.procreepy.procreate
└── No Timelapse.procreate              (omitido con una advertencia — nada escrito)
```
- **Nombres**: `<nombre original sin .procreate>`, con `.mp4` o
  `.procreepy.procreate` añadido. Los espacios, el cirílico y los caracteres
  especiales se conservan tal cual; en un sistema de archivos que no distingue
  entre mayúsculas y minúsculas, los nombres que difieren solo en esa clase
  reciben los sufijos `-2`, `-3`, …
- **Carpeta de salida**: por defecto `output/`, relativa al directorio actual,
  se crea automáticamente. Se puede indicar otra como segundo argumento:
  `procreepy input/ ~/Videos/procreate`.
- **El proyecto aligerado**: `projects/NAME.procreepy.procreate` es el mismo
  archivo sin los miembros `video/segments/segment-N.mp4`; todos los demás
  miembros se conservan byte a byte (orden, métodos de compresión, marcas de
  tiempo). El proyecto conserva la fecha y hora de modificación de su origen,
  de modo que volver a importarlo en Procreate no desordena la galería.
- **Volver a ejecutar es seguro**: una entrada cuyo timelapse y proyecto ya
  existen se omite; una pareja a medias se reconstruye en su totalidad. Para
  reconstruirlo todo: `--force` (`-f`).
- **`-r`** también desciende a las subcarpetas; su estructura se replica en
  ambos árboles, por lo que los nombres idénticos en carpetas distintas no
  entran en conflicto.
- **Un archivo defectuoso no detiene los demás.** Un archivo sin timelapse (la
  grabación estaba desactivada) es una advertencia, no un error: para él no se
  escribe nada. Un archivo corrupto es un error: aparece en el resumen final y
  el código de salida pasa a ser `1`.
- **Conjuntos atómicos**: las salidas de una entrada (timelapse + proyecto, y
  con `--psd` también el PSD) se escriben en archivos temporales y se publican
  juntos, o no se publican en absoluto. Una ejecución fallida nunca deja un
  vídeo huérfano junto a un proyecto que falta.
- Los archivos ocultos (`._Foo.procreate`, que macOS deja al copiar) se ignoran.
- Los originales nunca se modifican.

Ejemplo de salida (todo va a stderr; la ejecución termina con código de salida
`1` por el archivo corrupto):

```text
level=INFO msg="batch conversion started" files=4 input=input/ timelapses=output/timelapses/ projects=output/projects/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate" removed_segments=17 video_size="6.7 MiB"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" timelapse="output/timelapses/Portrait of a Cat.mp4" project="output/projects/Portrait of a Cat.procreepy.procreate" removed_segments=18 video_size="4.1 MiB"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`removed_segments` es el número de archivos de segmento retirados del proyecto
aligerado, y `video_size` su tamaño total (comprimido) dentro del archivo.
Ejecutar de nuevo el mismo comando informa, para cada pareja existente:

```text
level=INFO msg="skipped, outputs already exist (use --force to overwrite)" input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate"
```

`--list` y `--verify` también aceptan un directorio y recorren todos sus archivos.

### Exportación PSD (`--psd`)

```bash
procreepy --psd input/ out/
```

Solo entrada de directorio. Junto a cada pareja convertida se escribe
`out/psd/NAME.psd`, publicado de forma atómica con el resto del conjunto:

```text
level=INFO msg="psd exported" input="input/Portrait of a Cat.procreate" psd="out-psd/psd/Portrait of a Cat.psd" layers=3
```

Lo que incluye el PSD:

- el árbol de capas (grupos, orden), nombres de capa (Unicode), visibilidad,
  opacidad, modos de fusión, límites y estado de bloqueo;
- píxeles RGBA de 8 bits para cada capa, comprimidos con PackBits;
- los DPI y el perfil ICC incrustado;
- un compuesto fusionado tomado tal cual del propio render aplanado de
  Procreate (cuando este falta o está dañado, las capas visibles se componen en
  modo Normal como aproximación).

Lo que no incluye — el PSD es una exportación, no un ida y vuelta sin
pérdidas:

- las máscaras de capa y la semántica exacta del clip sobre la capa inferior no
  sobreviven;
- las capas de texto conservan sus píxeles, pero no sus datos de texto
  editables;
- el alfa directo (no premultiplicado) no puede recuperarse con exactitud:
  Procreate almacena teselas de 8 bits premultiplicadas, por lo que los colores
  de contorno en los bordes de las capas pueden diferir ligeramente;
- los lienzos más anchos o altos de 30000 píxeles se rechazan de plano (límite
  del formato PSD, a diferencia de PSB).

El proyecto `.procreate` sigue siendo la copia maestra; trata el PSD como un
instantáneo para Photoshop y otros importadores.

### Diagnóstico

```bash
procreepy --list artwork.procreate
```

```text
input: input/Portrait of a Cat.procreate
segments: 18

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
3  video/segments/segment-3.mp4
...
17 video/segments/segment-17.mp4
18 video/segments/segment-18.mp4
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
| `-h`, `--help` | mostrar la ayuda y salir |
| `--list` | listar los segmentos en orden de reproducción y salir |
| `--verify` | comprobar cada segmento; no crear ningún vídeo de salida |
| `-r`, `--recursive` | con una carpeta de entrada: recorrer también las subcarpetas |
| `-f`, `--force` | con una carpeta de entrada: sobrescribir las salidas que ya existen |
| `--strict` | tratar los números de segmento que faltan como un error (por defecto, advertencia) |
| `--psd` | con una carpeta de entrada: exportar además un PSD con capas por obra |
| `--tmpdir DIR` | dónde colocar los archivos temporales (por defecto: `$TMPDIR`, luego `/var/tmp`, luego el directorio temporal del sistema) |
| `-q`, `--quiet` | mostrar solo advertencias y errores |
| `--version` | mostrar el número de versión y salir |
| `--` | detener el análisis de opciones; tratar el resto como posicional |

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
7. Todo lo que la ejecución promete (el MP4, el proyecto aligerado, el PSD) se
   prepara en archivos temporales y se publica como un único conjunto solo
   después de que el último tenga éxito; en modo por lotes, la siguiente
   entrada se intenta de todos modos.
8. El archivo temporal, si existe, se elimina siempre — tras un éxito, un error,
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
  * Particularidad de Windows: el renombrado final es un `MoveFileEx` con
    reemplazo del destino existente, de modo que `--force` y la regeneración
    de un conjunto incompleto sustituyen los archivos de salida existentes
    in situ, igual que en Unix. No es estrictamente atómico como el rename de
    POSIX; la diferencia práctica aparece en un solo caso: si el destino
    sigue abierto en otro programa (por ejemplo, un reproductor con el MP4
    anterior abierto), el renombrado se rechaza con un error legible
    `Access is denied`, el archivo antiguo queda intacto y una nueva
    ejecución después de cerrar el programa finaliza con éxito.
  * stdout nunca se contamina con texto. Todas las líneas de registro
    (`level=INFO`/`WARN`/`ERROR`, un registro estructurado key=value por línea)
    van a stderr. La única excepción es el informe `--list`/`--verify`, donde
    stdout es el resultado. Si stdout es un terminal, la utilidad se niega a
    volcar en él un MP4 binario.

### Colores de consola

Cuando stderr es un terminal interactivo y la variable de entorno `NO_COLOR`
no está establecida, los tokens de nivel `WARN` y `ERROR` se pinta (en
amarillo y en rojo en negrita); `INFO` queda sin color. Las tuberías, las
redirecciones, la CI y las pruebas conservan el formato plano byte a byte, por
lo que nada que esté scripteado cambia. Deliberadamente no existe la opción
`--color`.

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
| 2 | argumentos incorrectos (incluido `--psd` con un solo archivo, o un directorio de resultados dirigido a stdout); OUTPUT es el mismo archivo que INPUT; stdout es un terminal |
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

El camino del timelapse no analiza `Document.archive` (NSKeyedArchive), no
toca `*.lz4`, no restaura capas ni renderiza la imagen; `--psd` sí analiza el
documento — para la exportación descrita arriba, con las limitaciones de
fidelidad descritas allí. Si el timelapse no se grabó en el archivo, esta
utilidad no puede recuperarlo del historial de dibujo. Nota: `lz4 -t` sobre un
`.lz4` extraído de un `.procreate` no es una comprobación de integridad — no
son fotogramas LZ4 independientes.

## Referencias del formato

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Licencia

Apache License 2.0, véase `LICENSE`.

---

## Idiomas

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
