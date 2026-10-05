# procreepy

Convierte en un MP4 normal el timelapse que Procreate ya grabó dentro de tu
archivo `.procreate`.

- Sin recodificar: los fotogramas se copian, así que el vídeo conserva la calidad de Procreate.
- Tus archivos `.procreate` originales nunca se modifican.
- Un archivo o una carpeta entera de una vez.
- Un solo programa. Sin ffmpeg, sin Python, sin cuenta, sin nada que configurar.
- Funciona sin conexión en Windows, macOS y Linux.

## ¿Es para ti?

Usa procreepy si:

- tienes archivos `.procreate`;
- la **grabación de timelapse estaba activada** mientras dibujabas (en Procreate
  lo está por defecto);
- quieres ese timelapse como un MP4 que puedas subir, editar o guardar;
- o quieres quitar los datos del timelapse de tus proyectos para que ocupen menos.

procreepy **no puede**:

- crear un timelapse que nunca se grabó: solo extrae uno existente;
- reconstruir un timelapse a partir de tus capas o del historial de deshacer;
- reparar un archivo `.procreate` dañado;
- exportar una imagen final de tu obra (sí puede exportar un PSD por capas, ver
  [Exportar a PSD](#exportar-a-psd)).

¿No sabes si tu archivo tiene timelapse?
[Compruébalo primero](#comprobar-un-archivo-antes-de-convertirlo): es un comando
y no crea nada.

## Qué obtienes

Un archivo de entrada, un vídeo de salida:

```text
my-art.procreate  →  procreepy  →  my-art.mp4
```

Una carpeta de entrada, dos carpetas de salida:

```text
input/                          output/
├── Cat.procreate         →     ├── timelapses/
├── Landscape.procreate   →     │   ├── Cat.mp4
└── Sketch.procreate      →     │   ├── Landscape.mp4
                                │   └── Sketch.mp4
                                └── projects/
                                    ├── Cat.procreepy.procreate
                                    ├── Landscape.procreepy.procreate
                                    └── Sketch.procreepy.procreate
```

- `timelapses/` contiene los vídeos.
- `projects/` contiene una copia de cada obra **con el timelapse eliminado**:
  mucho más pequeña, y puedes volver a importarla en Procreate. Todo lo demás del
  proyecto se conserva byte a byte.
- `input/` queda exactamente como estaba.

## Instalación

Descarga un programa ya compilado para tu sistema desde la página de releases;
no necesitas Go ni herramientas de compilación.

- GitLab: https://gitlab.com/po1nt-1/procreepy/-/releases
- GitHub: https://github.com/po1nt-1/procreepy/releases

Elige el archivo que corresponda a tu ordenador:

| Tu sistema | Descarga |
|---|---|
| Windows (la mayoría de PC) | `procreepy_<version>_windows_amd64.zip` |
| Windows en ARM | `procreepy_<version>_windows_arm64.zip` |
| Mac con Apple Silicon (M1–M5) | `procreepy_<version>_darwin_arm64.tar.gz` |
| Mac con Intel | `procreepy_<version>_darwin_amd64.tar.gz` |
| Linux (la mayoría de PC) | `procreepy_<version>_linux_amd64.tar.gz` |
| Linux en ARM (Raspberry Pi, Graviton) | `procreepy_<version>_linux_arm64.tar.gz` |
| Linux en ARM de 32 bits | `procreepy_<version>_linux_arm.tar.gz` |

En un Mac, menú Apple → «Acerca de este Mac» te dice si tienes Apple Silicon o
Intel.

### Windows

1. Descarga `procreepy_<version>_windows_amd64.zip`.
2. Clic derecho en el archivo descargado → **Extraer todo** → elige una carpeta
   que puedas encontrar después, por ejemplo `Downloads\procreepy`.
3. Abre esa carpeta, haz clic en la barra de direcciones de arriba, escribe `cmd`
   y pulsa Enter. Se abre una ventana negra de símbolo del sistema en esa carpeta.
4. Pon un archivo `.procreate` en la misma carpeta y ejecuta:

   ```text
   procreepy.exe "My Artwork.procreate" "My Artwork.mp4"
   ```

   Las comillas solo importan si el nombre tiene espacios.

**Sobre el aviso de SmartScreen.** El programa no está firmado con un
certificado de pago de Microsoft, así que la primera vez Windows puede mostrar
una ventana azul: «Windows protegió su PC», con una «aplicación no reconocida».
No es un aviso de virus: Windows lo muestra con cualquier programa que todavía no
ha visto suficientes veces. Para continuar, pulsa **Más información** y luego el
botón **Ejecutar de todas formas**. Si prefieres no hacerlo, usa la
[imagen de contenedor](#docker--podman).

### macOS

1. Descarga el `.tar.gz` de tu chip (`darwin_arm64` para Apple Silicon,
   `darwin_amd64` para Intel).
2. Abre Terminal (Aplicaciones → Utilidades → Terminal) y ve a tu carpeta de
   descargas:

   ```bash
   cd ~/Downloads
   ```

3. Descomprime y permite su ejecución:

   ```bash
   tar -xzf procreepy_*_darwin_*.tar.gz
   xattr -d com.apple.quarantine ./procreepy
   ```

   La línea `xattr` quita la marca de cuarentena de las descargas. Sin ella macOS
   se niega a arrancar el programa, porque no está notarizado por Apple.

4. Convierte un archivo:

   ```bash
   ./procreepy "My Artwork.procreate" "My Artwork.mp4"
   ```

Para poder escribir `procreepy` desde cualquier sitio, muévelo al PATH:
`sudo mv ./procreepy /usr/local/bin/`.

### Linux

```bash
tar -xzf procreepy_*_linux_amd64.tar.gz
./procreepy artwork.procreate artwork.mp4
```

El programa está enlazado estáticamente, así que funciona en cualquier
distribución independientemente de su versión de glibc. Para instalarlo para
todos los usuarios: `sudo install -m 755 procreepy /usr/local/bin/`.

### Docker / Podman

En cada release se publica una imagen de contenedor para `linux/amd64` y
`linux/arm64`. En un Mac con Apple Silicon se selecciona automáticamente la
variante arm64.

```bash
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4
```

`podman` sustituye a `docker` sin cambios. Para fijar una versión usa `:0.3.0` en
lugar de `:latest` (las etiquetas de imagen no llevan el prefijo `v`). Detalles
sobre la propiedad de los archivos y demás en
[usage.md](usage.md#container-usage).

### Compilar desde el código fuente

Solo es necesario si quieres cambiar el código. Ver
[development.md](development.md).

### Verificar la descarga (opcional)

Cada release publica también `CHECKSUMS.txt`. Para confirmar que la descarga
llegó íntegra, muestra el hash de tu archivo y compáralo con la línea
correspondiente:

```bash
sha256sum procreepy_0.3.0_linux_amd64.tar.gz    # Linux
shasum -a 256 procreepy_0.3.0_darwin_arm64.tar.gz   # macOS
grep darwin_arm64 CHECKSUMS.txt                 # el valor esperado
```

En Windows: `certutil -hashfile procreepy_0.3.0_windows_amd64.zip SHA256`.

Los dos valores deben ser idénticos. Es un paso opcional: detecta una descarga
truncada o manipulada, nada más.

## Convertir un archivo

```bash
procreepy artwork.procreate artwork.mp4
```

Resultado:

```text
artwork.mp4
```

El original `artwork.procreate` no se modifica. Si `artwork.mp4` ya existe, se
reemplaza, pero solo después de haber escrito el vídeo nuevo por completo.

También puedes darle una carpeta como destino y dejar que procreepy ponga el
nombre:

```bash
procreepy artwork.procreate videos/
```

Resultado: `videos/artwork.mp4`. La carpeta debe existir ya.

## Convertir una carpeta

```bash
procreepy input/
```

Lee todos los `.procreate` de `input/` y escribe en `output/`, como se muestra en
[Qué obtienes](#qué-obtienes). Para elegir el destino tú mismo:

```bash
procreepy input/ ~/Videos/timelapses
```

Para incluir subcarpetas (su estructura se refleja en la salida):

```bash
procreepy -r input/ output/
```

Qué ocurre mientras se ejecuta:

- El progreso se informa por archivo, una línea cada uno.
- Un archivo cuyo timelapse nunca se grabó se **omite con una advertencia**. No
  se escribe nada para él y la ejecución continúa.
- Un archivo dañado se informa como error, la ejecución sigue con el resto y el
  comando termina con código de salida `1` para que los scripts lo detecten.
- Ejecutar el mismo comando dos veces no repite el trabajo hecho: las obras cuyos
  resultados ya están se omiten. Añade `-f` para regenerarlas igualmente.

## Comprobar un archivo antes de convertirlo

Ninguno de estos comandos crea vídeo ni cambia nada.

**¿Hay un timelapse en este archivo, y de qué duración?**

```bash
procreepy --list artwork.procreate
```

```text
input: artwork.procreate
segments: 18

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
...
```

`--list` lee el índice del archivo. Es instantáneo y te dice si hay timelapse.

**¿Funcionará realmente la conversión?**

```bash
procreepy --verify artwork.procreate
```

`--verify` va más allá: lee cada segmento del timelapse, comprueba si está
dañado y confirma que los segmentos se pueden unir sin recodificar. Más lento que
`--list`, y la respuesta honesta a «¿esto se convertirá bien?».

Ambos aceptan también una carpeta, y entonces informan de cada archivo que
contiene.

## Exportar a PSD

```bash
procreepy --psd input/ output/
```

Junto a cada vídeo y proyecto ligero se escribe `output/psd/NAME.psd`: un archivo
de Photoshop por capas que puedes abrir en Photoshop, Affinity Photo, GIMP y
similares.

`--psd` funciona **solo con una carpeta de entrada**. Con un archivo suelto se
detiene con `--psd needs a directory INPUT; it writes into OUTPUT/psd/`.

El PSD es una exportación, no una copia perfecta. Conserva el árbol de capas y
sus nombres, la visibilidad, la opacidad, los modos de fusión y la propia imagen;
**no** conserva las máscaras de capa, las relaciones de recorte ni el texto
editable. Lee [qué conserva y qué pierde el PSD](usage.md#export-a-psd) antes de
usarlo para trabajo final. Guarda el archivo `.procreate` como copia maestra.

## Qué ocurre con tus archivos

- **Tus originales nunca se modifican.** procreepy abre los `.procreate` en modo
  de solo lectura. Todo lo que produce se escribe en otro sitio.
- **Nada queda a medio escribir.** Cada resultado se construye primero en un
  archivo temporal y se coloca en su sitio solo cuando está completo. Una
  ejecución interrumpida o fallida nunca deja un vídeo roto ni daña un archivo
  que ya existía.
- **Las ejecuciones por carpeta publican por obra, como conjunto.** El vídeo, el
  proyecto ligero y el PSD de una obra aparecen juntos o no aparecen: nunca
  obtienes un vídeo sin su proyecto.
- **Repetir la ejecución es seguro.** Las obras terminadas se omiten. Un conjunto
  que quedó incompleto por una ejecución interrumpida se regenera completo. `-f`
  regenera todo.
- **Convertir un archivo reemplaza el destino** si existe, después de escribir el
  vídeo nuevo por completo.
- **Los archivos grandes necesitan espacio temporal.** Los timelapses grandes se
  montan a través de un archivo temporal. Si te quedas sin espacio, apunta
  `--tmpdir` a un disco más holgado.

## Si algo va mal

| Lo que ves | Qué significa |
|---|---|
| `procreepy: command not found` | No estás en la carpeta donde lo descomprimiste; en macOS/Linux usa `./procreepy`. |
| `no video/segments in the archive` | En ese archivo no se grabó timelapse. No se puede recuperar. |
| `input is not a valid ZIP archive` | No es un archivo `.procreate`, o la descarga/copia está truncada. |
| `segment ... is corrupted inside the archive` | Los datos del timelapse están dañados. |
| `segments are incompatible` | El timelapse se grabó atravesando un cambio de lienzo o de calidad, así que no se puede unir sin recodificar. |
| `refusing to write video data to a terminal` | Añade un nombre de archivo de destino, o redirige con `> out.mp4`. |
| `Windows protegió su PC` | Ver [la nota sobre SmartScreen](#windows). |
| `no space left` / errores de escritura | Usa `--tmpdir` en un disco con más espacio libre. |

Cada uno de estos casos, con el síntoma exacto y qué hacer, está en
[troubleshooting.md](troubleshooting.md).

## Referencia de comandos

```text
procreepy [options] INPUT [OUTPUT]
```

`INPUT` es un archivo `.procreate`, una carpeta de ellos, o `-` para la entrada
estándar. `OUTPUT` es un nombre de archivo, una carpeta, o `-` para la salida
estándar. Para un archivo suelto, omitir `OUTPUT` escribe el vídeo en la salida
estándar; para una carpeta, el valor por defecto es `output/`.

| Opción | Qué hace | Se aplica a |
|---|---|---|
| `-h`, `--help` | mostrar la ayuda y salir | siempre |
| `--version` | mostrar la versión y salir | siempre |
| `--list` | listar los segmentos del timelapse; no escribir vídeo | archivo o carpeta |
| `--verify` | comprobar cada segmento; no escribir vídeo | archivo o carpeta |
| `-r`, `--recursive` | procesar también las subcarpetas | solo entrada de carpeta |
| `-f`, `--force` | sobrescribir resultados que ya existen | solo entrada de carpeta |
| `--psd` | exportar además un PSD por capas de cada obra | solo entrada de carpeta |
| `--strict` | tratar los huecos en la numeración de segmentos como errores, no advertencias | archivo o carpeta |
| `--tmpdir DIR` | dónde poner los archivos temporales | siempre |
| `-q`, `--quiet` | imprimir solo advertencias y errores | siempre |
| `--` | dejar de leer opciones; tratar el resto como nombres de archivo | siempre |

Referencia completa con ejemplos, formatos de salida y códigos de salida:
[usage.md](usage.md).

## Cómo funciona

Un archivo `.procreate` es un archivo ZIP. Cuando la grabación de timelapse está
activada, Procreate guarda el vídeo terminado dentro, dividido en trozos
numerados (`video/segments/segment-1.mp4`, `segment-2.mp4`, …). procreepy lee
esos trozos directamente del archivo, los ordena por número, comprueba que
comparten códec y lienzo, y los cose en un único MP4 copiando los fotogramas sin
tocarlos. No se renderiza ni se recodifica nada, y por eso es rápido y sin
pérdidas.

La ruta del timelapse nunca mira tus capas. Solo `--psd` lee la obra en sí.

Detalles — montaje del MP4, escritura atómica, estrategia de archivos temporales,
fidelidad del PSD: [how-it-works.md](how-it-works.md).

## Desarrollo

Compilación, pruebas, cobertura, compilación cruzada, release y CI:
[development.md](development.md).

Los requisitos son Go (la versión está en [`go.mod`](../go.mod)) y `make`. El
proyecto no tiene dependencias de terceros.

```bash
make check   # comprobación de formato + compilación + vet + toda la suite de pruebas
```

## Licencia

Apache License 2.0 — ver [LICENSE](../LICENSE).

## Idiomas

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
