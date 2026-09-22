# procreepy

Uma pequena utilidade Unix multiplataforma (Linux, Windows, macOS): extrai do
arquivo `.procreate` o timelapse de arquivo já pronto e junta seus segmentos
em um único MP4. Nada é re-codificado (stream copy), nada é renderizado.

`.procreate` é um ZIP. Se a gravação do timelapse estava ativada, dentro há

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

A utilidade pega exatamente estes arquivos: ordena-os **numericamente**
(`segment-9` antes de `segment-10`), analisa a estrutura MP4 de cada
segmento e os reconstrói em um MP4 moov-first copiando os quadros como
estão. Ela não abre `Document.archive`, camadas nem chunks raster
(`*.lz4`).

## Requisitos

Sem dependências externas: nem `ffmpeg` nem `ffprobe` são
necessários. Para compilar, só é preciso o Go (a versão está no `go.mod`).

```bash
go build -o procreepy ./cmd/procreepy    # or: make bin
```

### Compilar para outros sistemas operacionais

O projeto é Go puro e compila em modo cruzado sem problemas para todos os
destinos suportados. De qualquer plataforma, qualquer um destes comandos
funciona:

| Destino | Comando |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM de 64 bits (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM de 32 bits | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM de 64 bits | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |

Todas as compilações são estáticas (sem cgo): um binário Linux roda em
qualquer distribuição, seja qual for a versão da glibc. A pipeline do
GitLab CI compila exatamente estes destinos em cada commit; o job `dist`
publica os tarballs com um manifesto `SHA256SUMS`, e os jobs `repro:*`
provam que os binários são reproduzíveis bit a bit.

- Linux/macOS: não há passo de instalação, execute o binário diretamente.
- Windows: o binário não é assinado, então o SmartScreen pode exibir
  «Seu PC foi protegido» — escolha **Mais informações → Executar assim
  mesmo**.

## Uso

Um arquivo:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```

As quatro combinações `INPUT`/`OUTPUT` são suportadas: `FILE OUTPUT`,
`FILE -`, `- OUTPUT`, `- -`. Se `OUTPUT` for omitido, é o stdout. Se
`OUTPUT` for um diretório existente, o vídeo vai para lá com o nome
original (`procreepy art.procreate videos/` → `videos/art.mp4`).

### Modo em lote: uma pasta de `.procreate` → uma pasta de vídeos

O cenário "tenho `input/` cheia de `.procreate` e quero os vídeos em
`output/timelaps/`":

```bash
procreepy input/
```

```text
input/                             output/timelaps/
├── Portrait of a Cat.procreate →  ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →  ├── Landscape v2.mp4
└── No Timelapse.procreate            └── (pulado, com aviso)
```

- **Nomes**: `<nome original sem .procreate>.mp4`. Espaços, cirílico e
  caracteres especiais são preservados como estão.
- **Pasta de saída**: por padrão `output/timelaps/` relativo ao diretório
  atual, criada automaticamente. Outra pode ser dada como segundo
  argumento: `procreepy input/ ~/Videos/procreate`.
- **Reexecutar é seguro**: vídeos que já existem são pulados. Para
  reconstruir tudo: `--force` (`-f`).
- **`-r`** também desce subpastas; a estrutura das subpastas é espelhada no
  resultado (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`),
  assim nomes iguais em pastas diferentes não conflitam.
- **Um arquivo ruim não para os demais.** Arquivo sem timelapse (a gravação
  estava desligada) é um aviso, não um erro. Arquivo corrompido é erro: ele
  aparece no resumo final e o código de saída fica `1`.
- Arquivos ocultos (`._Foo.procreate`, que o macOS deixa para trás ao
  copiar) são ignorados.
- Os originais nunca são modificados.

Saída de exemplo (tudo vai para o stderr):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`--list` e `--verify` também aceitam um diretório e percorrem todos os
arquivos.

### Divisão: vídeo + projeto enxuto (`--split`)

A ideia: timelapses ocupam mais espaço que o próprio desenho — por exemplo,
ao fazer backup em um iPad é bom mantê-los separados. `--split` escreve, ao
lado de cada `MP4` finalizado, uma cópia enxuta do projeto **sem** nada sob
`video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (o timelapse, lossless)
                     →  artwork.procreepy.procreate  (o mesmo projeto, sem video/)
```

No modo em lote, o mesmo: ao lado de cada `X.mp4` surge um
`X.procreepy.procreate`. Todos os outros membros do arquivo (camadas,
`Info.plist`, prévias) são levados byte a byte: ordem, métodos de
compressão e timestamps são preservados. O `.procreate` original não é
modificado; a cópia enxuta não pode ser escrita no stdout, então `--split`
exige uma `OUTPUT` de arquivo.

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

Analisa cada segmento direto do arquivo (incluindo a checagem de CRC dentro
do ZIP), imprime um relatório linha a linha e confere que os segmentos
podem ser juntados sem re-codificação. **Nenhum vídeo de saída é criado.**
`--list` só lê o diretório do ZIP.

## Opções

| Opção | O que faz |
|---|---|
| `-r`, `--recursive` | entrada diretório: percorrer também subpastas |
| `-f`, `--force` | entrada diretório: sobrescrever vídeos que já existem |
| `--strict` | tratar números de segmento ausentes como erro (padrão é aviso) |
| `--reencode` | aceita por compatibilidade com scripts antigos; não há re-codificação, sempre stream copy |
| `--split` | escrever `X.procreepy.procreate` ao lado de cada `MP4` — o projeto sem `video/` |
| `--tmpdir DIR` | onde descompactar os segmentos |
| `-q`, `--quiet` | imprimir apenas avisos e erros |

## Como funciona

1. `INPUT` é um arquivo ou stdin. Stdin (e qualquer entrada não
   pesquisável) primeiro é despejado em um arquivo temporário, porque o ZIP
   requer acesso aleatório.
2. O ZIP é validado (somente leitura) e as entradas
   `video/segments/segment-N.mp4` são localizadas.
3. Ordenação numérica. Falhas na numeração são aviso; nomes sem número são
   ignorados com aviso.
4. Cada segmento é analisado direto do ZIP (sem extração completa): boxes
   MP4, tamanhos dos fluxos, parâmetros do codec. No primeiro dano — parar.
5. Checagem de compatibilidade (resolução, codec, conjuntos SPS/PPS,
   áudio). Senão `-c copy` silenciosamente daria lixo — por isso a
   incompatibilidade é um erro com mensagem clara, não uma surpresa no
   vídeo final.
6. O MP4 moov-first é montado: `ftyp`, `moov` (todos os trilhas, fatiados
   dos segmentos), então `mdat` após `mdat` em ordem de reprodução.
7. O arquivo temporário (se houver) é removido sempre — em sucesso, em
   erro, em Ctrl+C e em SIGTERM.

### Escrever para arquivo e para stdout

Os dois caminhos montam o mesmo MP4 moov-first: o átomo moov é escrito
primeiro porque os quadros são copiados direto dos segmentos de origem e os
metadados são conhecidos antes de começar a escrever. Para arquivo, é um
MP4 "clássico", servível tanto para players quanto para editores; o mesmo
arquivo exato vai para o pipe — `> artwork.mp4` dá o mesmo resultado que um
`procreepy artwork.procreate artwork.mp4` explícito.

- Saída para **arquivo** é atômica: um `.partial` ao lado do alvo,
  renomeado somente após o sucesso. Uma execução falha não deixa restos e
  nunca corrompe um arquivo existente.
- O stdout nunca é sujo com texto. Todos os registros
  `level=INFO`/`WARN`/`ERROR` (um registro estruturado key=value por linha)
  vão para o stderr. A única exceção é o relatório
  `--list`/`--verify`, onde o stdout *é* o resultado. Se o stdout é um
  terminal, a utilidade se recusa a despejar um MP4 binário nele.

### Arquivos temporários e Fedora

No Fedora, `/tmp` é um tmpfs na memória. Os segmentos do timelapse podem
ter centenas de megabytes, e lendo do stdin a `.procreate` inteira é
despejada. Por isso a pasta temporária é escolhida assim: `--tmpdir` →
`$TMPDIR` → `/var/tmp` (em disco). O espaço livre é checado antes da
extração; se faltar, sai um erro claro com dica, e não um "No space left"
no meio do trabalho.

## Códigos de saída

| Código | Significado |
|---|---|
| 0 | sucesso |
| 1 | erro inesperado; no modo em lote — pelo menos um arquivo falhou |
| 2 | argumentos inválidos; a saída sobrescreveria a entrada; stdout é um terminal |
| 3 | entrada não encontrada, vazia ou não é ZIP |
| 4 | nenhum `video/segments` no arquivo (não havia timelapse gravado) |
| 5 | segmento corrompido; numeração ambígua ou ausente (`--strict`) |
| 6 | reservado (não usado: sem dependências externas) |
| 7 | segmentos incompatíveis para stream copy |
| 8 | reservado (não usado: sem dependências externas) |
| 9 | falha ao escrever o resultado ou arquivos temporários |
| 130 | interrompido (Ctrl+C / SIGTERM) |

## Testes

```bash
go test ./...
```

Arquivos `.procreate` reais não são necessários: os testes constroem ZIPs a
partir de segmentos MP4 gerados (veja `internal/testkit`). As verificações
são estruturais: análise do MP4 resultante, ordem das boxes, número de
amostras, conteúdo do `mdat`. `-race` não é necessário, mas funciona se
houver um compilador C instalado.

Coberto: arquivo comum, ausência de `video/segments`, um segmento só,
segmentos fora de ordem (`segment-9`/`segment-10`), stdin, stdout, espaços
e caracteres especiais em nomes, ZIPs corrompidos e truncados, MP4s
corrompidos e truncados, dano de CRC, erros de escrita (`/dev/full`),
segmentos incompatíveis e o modo em lote inteiro.

## O que a utilidade deliberadamente não faz

Não analisa `Document.archive` (NSKeyedArchive), não mexe em `*.lz4`, não
restaura camadas e não renderiza a imagem. Se o timelapse não foi gravado
no arquivo, esta utilidade não consegue recuperá-lo do histórico do
desenho. Nota: `lz4 -t` num `.lz4` tirado de um `.procreate` não é uma
checagem de integridade — eles não são frames LZ4 independentes.

## Referências de formato

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Licença

Apache License 2.0, veja `LICENSE`.

---

## Idiomas

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
