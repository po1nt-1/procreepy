# procreepy

Um pequeno utilitário multiplataforma (Linux, Windows, macOS): extrai do arquivo
`.procreate` o timelapse já pronto e junta seus segmentos em um único MP4. Nada
é recodificado (stream copy) e nada é renderizado.

`.procreate` é um arquivo ZIP. Se a gravação do timelapse estava ativada, ele
contém

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
O utilitário pega exatamente esses arquivos: ordena-os **numericamente**
(`segment-9` antes de `segment-10`), analisa a estrutura MP4 de cada segmento
e os reconstrói em um único MP4 moov-first, copiando os quadros como estão. Ele
nunca abre `Document.archive`, camadas ou chunks raster (`*.lz4`).

## Requisitos

Não há dependências externas: não são necessários `ffmpeg` nem `ffprobe`. A compilação requer apenas Go (a versão está em `go.mod`) e `make` (presente em todas as plataformas suportadas; em sistemas mínimos, está a um comando do gerenciador de pacotes).
O Makefile é o ponto de entrada canônico da compilação: fixa o mesmo ambiente hermético usado pela CI (modo offline para módulos, toolchain local, sem cgo) e localiza automaticamente a toolchain do Go.

```bash
make build      # compilar tudo e gerar um ./procreepy executável
make check      # gofmt + compilação + vet + suíte completa de testes
```

`make build` grava `dev-<commit>` no binário, para que `procreepy --version` informe de qual commit ele veio (os tarballs de release levam a tag no lugar). Sem `make`, o equivalente direto é `go build ./... && go build -o procreepy ./cmd/procreepy` (esse binário informa `dev`, ou `dev-<commit>` quando é compilado dentro de um checkout do Git).

### Compilar para outros sistemas operacionais

O projeto é escrito em Go puro e compila por cross-compilation para todos os destinos suportados. De qualquer plataforma:

| Destino | Comando |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM de 64 bits (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM de 32 bits | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM de 64 bits | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |
Cada destino produz um `dist/procreepy-<version>-<os>-<arch>.tar.gz` normalizado e imprime seu SHA-256; `make cross` compila toda a matriz de uma vez, e `make repro` comprova que uma compilação é reproduzível bit a bit. O equivalente direto para um destino é `GOOS=… GOARCH=… go build -o procreepy[.exe] ./cmd/procreepy`.
Todas as compilações são estáticas (sem cgo): um binário Linux funciona em qualquer distribuição, independentemente da versão do glibc. Os pipelines de CI (GitLab e GitHub) compilam exatamente esses destinos a cada commit; o job `dist` publica os tarballs e o manifesto `SHA256SUMS`, e os jobs `repro:*` comprovam a reprodutibilidade bit a bit dos binários.

- Linux/macOS: nenhuma instalação é necessária; execute o binário diretamente.
- Windows: o binário não é assinado, então o SmartScreen pode exibir “O Windows protegeu o computador” — escolha **Mais informações → Executar assim mesmo**.

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
As quatro combinações `INPUT`/`OUTPUT` são suportadas: `FILE OUTPUT`, `FILE -`,
`- OUTPUT`, `- -`. Se `OUTPUT` for omitido, a saída é stdout. Se `OUTPUT` for um
diretório existente, o vídeo é colocado nele com o nome original
(`procreepy art.procreate videos/` → `videos/art.mp4`).

### Modo em lote: uma pasta de `.procreate` → uma pasta de vídeos

O cenário «tenho `input/` cheio de arquivos `.procreate` e quero os vídeos em
`output/timelaps/`»:

```bash
procreepy input/
```

```text
input/                             output/timelaps/
├── Portrait of a Cat.procreate →  ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →  ├── Landscape v2.mp4
└── No Timelapse.procreate            └── (ignorado, com um aviso)
```
- **Nomes**: `<nome original sem .procreate>.mp4`. Espaços, cirílico e caracteres
  especiais são preservados como estão.
- **Pasta de saída**: por padrão `output/timelaps/`, relativa ao diretório atual,
  criada automaticamente. Outra pode ser indicada como segundo argumento:
  `procreepy input/ ~/Videos/procreate`.
- **Executar novamente é seguro**: vídeos que já existem são ignorados. Para
  reconstruir tudo: `--force` (`-f`).
- **`-r`** também percorre subpastas; a estrutura é espelhada no resultado
  (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`), então nomes
  idênticos em pastas diferentes não entram em conflito.
- **Um arquivo com problema não interrompe os demais.** Um arquivo sem timelapse
  (a gravação estava desligada) é um aviso, não um erro. Um arquivo corrompido
  é um erro: aparece no resumo final e o código de saída passa a ser `1`.
- Arquivos ocultos (`._Foo.procreate`, deixados pelo macOS ao copiar) são ignorados.
- Os originais nunca são modificados.
Exemplo de saída (tudo vai para stderr):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```
`--list` e `--verify` também aceitam um diretório e percorrem todos os seus arquivos.

### Separação: vídeo + projeto enxuto (`--split`)

A ideia: timelapses ocupam mais espaço do que o próprio desenho — por exemplo,
ao fazer backup em um iPad, pode ser útil mantê-los separados. `--split`
escreve, ao lado de cada `MP4` concluído, uma cópia enxuta do projeto **sem**
nada abaixo de `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (o timelapse, lossless)
                     →  artwork.procreepy.procreate  (o mesmo projeto, sem video/)
```
No modo em lote, acontece o mesmo: ao lado de cada `X.mp4` aparece um
`X.procreepy.procreate`. Todos os outros membros do arquivo (camadas,
`Info.plist`, prévias) são copiados byte a byte: ordem, métodos de compressão e
marcas de tempo são preservados. O `.procreate` original não é modificado; a
cópia enxuta não pode ser escrita em stdout, então `--split` exige um `OUTPUT`
de arquivo.

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
Analisa cada segmento diretamente do arquivo (incluindo a verificação de CRC
dentro do ZIP), imprime um relatório linha a linha e verifica se os segmentos
podem ser unidos sem recodificação. **Nenhum vídeo de saída é criado.** `--list`
apenas lê o diretório do ZIP.

## Opções

| Opção | O que faz |
|---|---|
| `-r`, `--recursive` | entrada como diretório: percorrer também as subpastas |
| `-f`, `--force` | entrada como diretório: sobrescrever vídeos que já existem |
| `--strict` | tratar números de segmentos ausentes como erro (por padrão, aviso) |
| `--reencode` | aceito por compatibilidade com scripts antigos; não há recodificação, sempre stream copy |
| `--split` | escrever `X.procreepy.procreate` ao lado de cada `MP4` — o projeto sem `video/` |
| `--tmpdir DIR` | onde colocar os arquivos temporários |
| `-q`, `--quiet` | imprimir apenas avisos e erros |

## Como funciona

1. `INPUT` é um arquivo ou stdin. Stdin (e qualquer entrada não-seekable) é
   primeiro armazenado em um arquivo temporário, porque ZIP exige acesso aleatório.
2. O ZIP é validado (somente leitura), e as entradas
   `video/segments/segment-N.mp4` são localizadas.
3. Ordenação numérica. Falhas na numeração são um aviso; nomes sem número são
   ignorados com um aviso.
4. Cada segmento é analisado diretamente do ZIP (sem extração completa): caixas
   MP4, tamanhos das faixas, parâmetros do codec. Na primeira corrupção, para.
5. Verificação de compatibilidade (resolução, codec, conjuntos SPS/PPS, áudio).
   Caso contrário, `-c copy` produziria dados inválidos silenciosamente — por
   isso a incompatibilidade é um erro com uma mensagem clara, não uma surpresa
   no vídeo final.
6. O MP4 moov-first é montado: `ftyp`, `moov` (todas as faixas, recortadas dos
   segmentos) e depois `mdat` após `mdat`, na ordem de reprodução.
7. O arquivo temporário (se existir) é sempre removido — em caso de sucesso,
   erro, Ctrl+C e SIGTERM.

### Escrever em arquivo e em stdout

Nos dois caminhos, o mesmo MP4 moov-first é montado: o átomo moov é escrito
primeiro porque os quadros são copiados diretamente dos segmentos de origem e
os metadados já são conhecidos antes de começar a escrever. Para um arquivo,
é um MP4 «clássico», adequado tanto para players quanto para editores; o mesmo
arquivo exato vai para um pipe — `> artwork.mp4` produz o mesmo resultado que
`procreepy artwork.procreate artwork.mp4` explicitamente.
  * **A saída para arquivo** é atômica: um `.partial` é criado ao lado do destino
    e só é renomeado após o sucesso. Uma execução malsucedida não deixa restos
    nem corrompe um arquivo existente.
  * stdout nunca é misturado com texto. Todas as linhas de log
    (`level=INFO`/`WARN`/`ERROR`, um registro estruturado key=value por linha)
    vão para stderr. A única exceção é o relatório `--list`/`--verify`, onde
    stdout é o resultado. Se stdout for um terminal, o utilitário se recusa a
    despejar um MP4 binário nele.

### Arquivos temporários e Fedora

No Fedora, `/tmp` é um tmpfs na RAM. Os segmentos de timelapse podem ter centenas
de megabytes e, ao ler de stdin, todo o `.procreate` é armazenado. Por isso, o
diretório temporário é escolhido assim: `--tmpdir` → `$TMPDIR` → `/var/tmp` (em
disco) → o diretório padrão do sistema. Se o disco ficar cheio durante esse
armazenamento, você recebe um erro claro com uma indicação (usar `--tmpdir` em
um diretório maior, respaldado por disco) em vez de um simples «No space left».

## Códigos de saída

| Código | Significado |
|---|---|
| 0 | sucesso |
| 1 | erro inesperado; no modo em lote — pelo menos um arquivo falhou |
| 2 | argumentos inválidos; a saída sobrescreveria a entrada; stdout é um terminal |
| 3 | entrada não encontrada, vazia ou não é ZIP |
| 4 | não há `video/segments` no arquivo (nenhum timelapse foi gravado) |
| 5 | segmento corrompido; numeração ambígua ou ausente (`--strict`) |
| 6 | reservado (não usado: sem dependências externas) |
| 7 | segmentos incompatíveis para stream copy |
| 8 | reservado (não usado: sem dependências externas) |
| 9 | falha ao gravar o resultado ou os arquivos temporários |
| 130 | interrompido (Ctrl+C / SIGTERM) |

## Testes

```bash
make test          # or: go test ./...
```
Arquivos `.procreate` reais não são necessários: os testes constroem ZIPs a
partir de segmentos MP4 gerados (veja `internal/testkit`). As verificações são
estruturais: análise do MP4 resultante, ordem das caixas, número de amostras,
conteúdo de `mdat`. `-race` não é necessário, mas funciona se houver um
compilador C instalado.
Cobertura: arquivo comum, ausência de `video/segments`, um único segmento,
segmentos fora de ordem (`segment-9`/`segment-10`), stdin, stdout, espaços e
caracteres especiais nos nomes, ZIPs corrompidos e truncados, MP4s corrompidos e
truncados, danos de CRC, erros de gravação (`/dev/full`), segmentos incompatíveis
e todo o modo em lote.

## O que o utilitário deliberadamente não faz

Ele não analisa `Document.archive` (NSKeyedArchive), não toca em `*.lz4`, não
restaura camadas e não renderiza a imagem. Se o timelapse não foi gravado no
arquivo, este utilitário não pode recuperá-lo do histórico de desenho. Observação:
`lz4 -t` em um `.lz4` retirado de um `.procreate` não é uma verificação de
integridade — não são frames LZ4 independentes.

## Referências de formato

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Licença

Apache License 2.0, veja `LICENSE`.

---

## Idiomas

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
