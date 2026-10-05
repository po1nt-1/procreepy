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
e os reconstrói em um único MP4 moov-first, copiando os quadros como estão.
No modo em lote ele também escreve, para cada obra convertida, uma cópia
reimportável do projeto sem o timelapse e — com `--psd` — um PSD do Photoshop
com camadas. O caminho do timelapse nunca abre `Document.archive`, camadas ou
chunks raster (`*.lz4`); só `--psd` abre.

## Requisitos

Não há dependências externas: não são necessários `ffmpeg` nem `ffprobe`. A compilação requer apenas Go (a versão está em `go.mod`) e `make` (presente em todas as plataformas suportadas; em sistemas mínimos, está a um comando do gerenciador de pacotes).
O Makefile é o ponto de entrada canônico da compilação: fixa o mesmo ambiente hermético usado pela CI (modo offline para módulos, toolchain local, sem cgo) e localiza automaticamente a toolchain do Go.

```bash
make build      # compilar tudo e gerar um ./procreepy executável
make check      # gofmt + compilação + vet + suíte completa de testes
```

`make build` grava `dev-<short sha>` (fora de um checkout do Git: `dev-nogit`)
no binário, para que `procreepy --version` informe de onde ele veio (os
tarballs de release levam a tag no lugar). Sem `make`, o equivalente direto é
`go build ./... && go build -o procreepy ./cmd/procreepy` (esse binário
informa `dev`, ou `dev-<commit>` quando o stamping por VCS está disponível).

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
Todas as compilações são estáticas (sem cgo): um binário Linux funciona em qualquer distribuição, independentemente da versão do glibc. Os pipelines de CI (GitLab e GitHub) compilam exatamente esses destinos a cada commit e, como o Windows é a plataforma principal, executam além disso a suíte completa de testes contra o binário `windows/amd64` real: de forma nativa no runner hospedado do GitHub e sob Wine no GitLab, que só tem runners Linux (veja `ci/windows-wine/`); o job `dist` publica os tarballs e o manifesto `SHA256SUMS`, e os jobs `repro:*` comprovam a reprodutibilidade bit a bit dos binários.

- Linux/macOS: nenhuma instalação é necessária; execute o binário diretamente.
- Windows: o binário não é assinado, então o SmartScreen pode exibir “O Windows protegeu o computador” — escolha **Mais informações → Executar assim mesmo**.

## Execução em um contêiner

Cada tag de versão publica também uma imagem multiarquitetura (`linux/amd64`, `linux/arm64`) no registro do projeto, de modo que a CLI roda sem toolchain do Go e sem descompactar nenhum tarball. Isso cobre também o Apple Silicon: o Docker Desktop e o `podman machine` sobem uma máquina virtual `linux/arm64` em um Mac da série M, então a variante arm64 é escolhida automaticamente — sem a flag `--platform` e sem emulação.

```bash
# um único arquivo — monte o diretório atual e use caminhos dentro dele
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4

# modo em lote: uma pasta de entrada, uma pasta de saída
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest input/ output/

# stdin → stdout não precisa de nenhuma montagem
docker run --rm -i registry.gitlab.com/po1nt-1/procreepy:latest - \
  < artwork.procreate > artwork.mp4
```

`podman` substitui `docker` sem alterações. Para fixar uma versão use `:1.2.3` em vez de `:latest` (as tags da imagem não têm o prefixo `v`); `latest` nunca é movido para uma tag de pré-lançamento.

- **Propriedade dos arquivos.** A imagem roda como uid 65532 (`distroless/static:nonroot`). Em um host Linux, acrescente `--user "$(id -u):$(id -g)"` para que os arquivos de saída pertençam a você; o Docker Desktop no macOS mapeia a propriedade da montagem por conta própria e não precisa de nada.
- **Sem shell dentro.** A imagem contém apenas o binário estático, portanto `docker run … --help` ou `… --verify file.procreate` funcionam, mas não há `sh` para entrar.
- **Os arquivos temporários** ficam na camada gravável do contêiner, não na montagem. Um timelapse grande pode precisar de espaço ali; `--tmpdir /data/tmp` os move para o volume montado.
- **Projetos privados.** O acesso ao registro segue a visibilidade do projeto: em um projeto público o pull é anônimo, caso contrário execute primeiro `docker login registry.gitlab.com`.

## Uso

Um arquivo:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
```
As quatro combinações `INPUT`/`OUTPUT` são suportadas: `FILE OUTPUT`, `FILE -`,
`- OUTPUT`, `- -`. Se `OUTPUT` for omitido, a saída é stdout. Se `OUTPUT` for um
diretório existente, o vídeo é colocado nele com o nome original
(`procreepy art.procreate videos/` → `videos/art.mp4`).

### Modo em lote: uma pasta de `.procreate` → pastas de vídeos e projetos

O cenário «tenho `input/` cheio de arquivos `.procreate` e quero os resultados
em `output/`»:

```bash
procreepy input/
```

Cada obra convertida produz um par — o timelapse e um projeto reimportável
sem ele:

```text
input/                               output/
├── Portrait of a Cat.procreate  →   ├── timelapses/Portrait of a Cat.mp4
│                                    ├── projects/Portrait of a Cat.procreepy.procreate
├── Landscape v2.procreate       →   ├── timelapses/Landscape v2.mp4
│                                    └── projects/Landscape v2.procreepy.procreate
└── No Timelapse.procreate              (ignorado com um aviso — nada escrito)
```
- **Nomes**: `<nome original sem .procreate>`, com `.mp4` ou
  `.procreepy.procreate` acrescentado. Espaços, cirílico e caracteres
  especiais são preservados como estão; em um sistema de arquivos sem
  distinção de maiúsculas e minúsculas, nomes que diferem só por isso
  recebem os sufixos `-2`, `-3`, …
- **Pasta de saída**: por padrão `output/`, relativa ao diretório atual,
  criada automaticamente. Outra pode ser indicada como segundo argumento:
  `procreepy input/ ~/Videos/procreate`.
- **Projeto enxuto**: `projects/NAME.procreepy.procreate` é o mesmo arquivo
  sem os membros `video/segments/segment-N.mp4`; todos os demais membros
  são copiados byte a byte (ordem, métodos de compressão, marcas de tempo).
  O projeto conserva o horário de modificação da sua origem, para que
  reimportá-lo no Procreate não embaralhe a galeria.
- **Executar novamente é seguro**: uma entrada cujo timelapse e projeto já
  existem é ignorada; um par pela metade é reconstruído inteiro. Para
  reconstruir tudo: `--force` (`-f`).
- **`-r`** também percorre subpastas; sua estrutura é replicada nas duas
  árvores, então nomes idênticos em pastas diferentes não entram em conflito.
- **Um arquivo com problema não interrompe os demais.** Um arquivo sem
  timelapse (a gravação estava desligada) é um aviso, não um erro: para ele
  nada é escrito. Um arquivo corrompido é um erro: aparece no resumo final e
  o código de saída passa a ser `1`.
- **Conjuntos atômicos**: as saídas de uma entrada (timelapse + projeto, e o
  PSD com `--psd`) são gravadas em arquivos temporários e publicadas juntas —
  tudo ou nada. Uma execução malsucedida nunca deixa um vídeo órfão ao lado
  de um projeto que falta.
- Arquivos ocultos (`._Foo.procreate`, deixados pelo macOS ao copiar) são ignorados.
- Os originais nunca são modificados.

Exemplo de saída (tudo vai para stderr; a execução termina com o código de
saída `1` por causa do arquivo corrompido):

```text
level=INFO msg="batch conversion started" files=4 input=input/ timelapses=output/timelapses/ projects=output/projects/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate" removed_segments=17 video_size="6.7 MiB"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" timelapse="output/timelapses/Portrait of a Cat.mp4" project="output/projects/Portrait of a Cat.procreepy.procreate" removed_segments=18 video_size="4.1 MiB"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`removed_segments` é o número de arquivos de segmento retirados do projeto
enxuto, e `video_size` o tamanho total deles (comprimido) dentro do arquivo.
Executar de novo o mesmo comando informa, para cada par existente:

```text
level=INFO msg="skipped, outputs already exist (use --force to overwrite)" input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate"
```

`--list` e `--verify` também aceitam um diretório e percorrem todos os seus arquivos.

### Exportação PSD (`--psd`)

```bash
procreepy --psd input/ out/
```

Só entrada de diretório. Ao lado de cada par convertido é gravado
`out/psd/NAME.psd`, publicado atomicamente junto com o resto do conjunto:

```text
level=INFO msg="psd exported" input="input/Portrait of a Cat.procreate" psd="out-psd/psd/Portrait of a Cat.psd" layers=3
```

O que o PSD contém:

- a árvore de camadas (grupos, ordem), nomes das camadas (Unicode),
  visibilidade, opacidade, modos de mesclagem, limites e estado de bloqueio;
- pixels RGBA de 8 bits para cada camada, comprimidos com PackBits;
- os DPI e o perfil ICC incorporado;
- o composto mesclado, retomado tal qual do próprio render achatado do
  Procreate (quando esse falta ou está danificado, as camadas visíveis são
  compostas no modo Normal como aproximação).

O que não entra — o PSD é uma exportação, não uma ida e volta sem perdas:

- máscaras de camada e a semântica exata de clip na camada de baixo não
  sobrevivem;
- camadas de texto conservam os pixels, mas não os dados de texto
  editáveis;
- o alfa direto (não pré-multiplicado) não pode ser recuperado exatamente:
  o Procreate armazena tiles de 8 bits pré-multiplicados, então as cores de
  contorno nas bordas das camadas podem diferir levemente;
- lienzos mais largos ou altos que 30000 pixels são recusados de plano
  (limite do formato PSD, ao contrário do PSB).

O `.procreate` continua sendo a cópia mestra; trate o PSD como um instantâneo
para o Photoshop e outros importadores.

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
Analisa cada segmento diretamente do arquivo (incluindo a verificação de CRC
dentro do ZIP), imprime um relatório linha a linha e verifica se os segmentos
podem ser unidos sem recodificação. **Nenhum vídeo de saída é criado.** `--list`
apenas lê o diretório do ZIP.

## Opções

| Opção | O que faz |
|---|---|
| `-h`, `--help` | mostrar a ajuda e sair |
| `--list` | listar os segmentos em ordem de reprodução e sair |
| `--verify` | verificar cada segmento; não criar vídeo de saída |
| `-r`, `--recursive` | entrada como diretório: percorrer também as subpastas |
| `-f`, `--force` | entrada como diretório: sobrescrever resultados que já existem |
| `--strict` | tratar números de segmentos ausentes como erro (por padrão, aviso) |
| `--psd` | entrada como diretório: exportar além disso um PSD com camadas por obra |
| `--tmpdir DIR` | onde colocar os arquivos temporários (padrão: `$TMPDIR`, depois `/var/tmp`, depois o diretório temporário do sistema) |
| `-q`, `--quiet` | imprimir apenas avisos e erros |
| `--version` | mostrar o número da versão e sair |
| `--` | parar o processamento de opções; tratar o resto como posicional |

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
7. Tudo o que a execução promete (o MP4, o projeto enxuto, o PSD) é
   preparado em arquivos temporários e publicado como um único conjunto
   somente depois que o último tem sucesso; no modo em lote, a próxima
   entrada é tentada de qualquer maneira.
8. O arquivo temporário (se existir) é sempre removido — em caso de sucesso,
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
  * Especificidade do Windows: a renomeação final é um `MoveFileEx` com
    substituição do destino existente, de modo que `--force` e a regeneração
    de um conjunto incompleto substituem as saídas existentes no lugar,
    assim como no Unix. Não é estritamente atômico como o rename do POSIX; a
    diferença prática aparece em um único caso — se o destino ainda estiver
    aberto em outro programa (por exemplo, um player de mídia com o MP4
    anterior aberto), a renomeação é recusada com o erro legível
    `Access is denied`, o arquivo antigo permanece intocado e uma nova
    execução após fechar o programa termina com sucesso.
  * stdout nunca é misturado com texto. Todas as linhas de log
    (`level=INFO`/`WARN`/`ERROR`, um registro estruturado key=value por linha)
    vão para stderr. A única exceção é o relatório `--list`/`--verify`, onde
    stdout é o resultado. Se stdout for um terminal, o utilitário se recusa a
    despejar um MP4 binário nele.

### Cores no console

Quando o stderr é um terminal interativo e a variável de ambiente `NO_COLOR`
não está definida, os tokens de nível `WARN` e `ERROR` são destacados (em
amarelo e vermelho em negrito); `INFO` fica sem cor. Pipes, redirecionamentos,
a CI e os testes preservam o formato plain byte a byte, então nada que esteja
escripitado muda. Deliberadamente não existe a opção `--color`.

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
| 2 | argumentos inválidos (inclusive `--psd` com um arquivo único, ou um diretório de resultados direcionado ao stdout); OUTPUT é o mesmo arquivo que INPUT; stdout é um terminal |
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

O caminho do timelapse não analisa `Document.archive` (NSKeyedArchive), não
toca em `*.lz4`, não restaura camadas e não renderiza a imagem; `--psd`
analisa o documento — para a exportação descrita acima, com as ressalvas de
fidelidade daquele mesmo trecho. Se o timelapse não foi gravado no arquivo,
este utilitário não pode recuperá-lo do histórico de desenho. Observação:
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
