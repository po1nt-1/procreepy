# procreepy

Transforma em um MP4 comum o timelapse que o Procreate já gravou dentro do seu
arquivo `.procreate`.

- Sem recodificar: os quadros são copiados, então o vídeo mantém a qualidade do Procreate.
- Seus arquivos `.procreate` originais nunca são modificados.
- Um arquivo ou uma pasta inteira de uma vez.
- Um único programa. Sem ffmpeg, sem Python, sem conta, nada para configurar.
- Funciona offline no Windows, macOS e Linux.

## Isto é para você?

Use o procreepy se:

- você tem arquivos `.procreate`;
- a **gravação de timelapse estava ligada** enquanto você desenhava (no Procreate
  ela vem ligada por padrão);
- você quer esse timelapse como um MP4 para publicar, editar ou guardar;
- ou quer remover os dados do timelapse dos seus projetos para deixá-los menores.

O procreepy **não pode**:

- criar um timelapse que nunca foi gravado — ele apenas extrai um já existente;
- reconstruir um timelapse a partir das suas camadas ou do histórico de desfazer;
- reparar um arquivo `.procreate` danificado;
- exportar uma imagem final da sua arte (ele pode exportar um PSD em camadas,
  veja [Exportar para PSD](#exportar-para-psd)).

Não sabe se o seu arquivo tem timelapse?
[Verifique primeiro](#verificar-um-arquivo-antes-de-converter) — é um comando e
não cria nada.

## O que você obtém

Um arquivo de entrada, um vídeo de saída:

```text
my-art.procreate  →  procreepy  →  my-art.mp4
```

Uma pasta de entrada; na saída, uma árvore de vídeos e um arquivo de projetos:

```text
input/                          input_procreepy/
├── Cat.procreate         →     ├── mp4/
├── Landscape.procreate   →     │   ├── Cat.mp4
└── Sketch.procreate      →     │   ├── Landscape.mp4
                                │   └── Sketch.mp4
                                └── procreate.zip   (os projetos enxutos)
```

- `mp4/` contém os vídeos.
- `procreate.zip` contém uma cópia de cada arte **com o timelapse removido**:
  muito menor, e você pode importá-la de volta no Procreate. Todo o resto do
  projeto é preservado byte a byte. É um zip comum com uma pasta `procreate/`
  dentro, pronto para passar para um iPad; use `--no-zip` para deixar essa pasta
  `procreate/` em disco em vez do arquivo.
- A pasta de saída recebe o nome da de entrada (`input/` → `input_procreepy/`) e é
  criada no diretório de onde você executa o comando. Informe um segundo caminho
  para escolhê-la você mesmo.
- Cada saída mantém a data do arquivo de origem, de modo que reimportar um projeto
  no Procreate não reorganiza sua galeria.
- `input/` fica exatamente como estava.

## Instalação

Baixe um programa pronto para o seu sistema na página de releases — você não
precisa de Go nem de ferramentas de compilação.

- GitLab: https://gitlab.com/po1nt-1/procreepy/-/releases
- GitHub: https://github.com/po1nt-1/procreepy/releases

Escolha o arquivo que corresponde ao seu computador:

| Seu sistema | Download |
|---|---|
| Windows (a maioria dos PCs) | `procreepy_<version>_windows_amd64.zip` |
| Windows em ARM | `procreepy_<version>_windows_arm64.zip` |
| Mac com Apple Silicon (M1–M5) | `procreepy_<version>_darwin_arm64.tar.gz` |
| Mac com Intel | `procreepy_<version>_darwin_amd64.tar.gz` |
| Linux (a maioria dos PCs) | `procreepy_<version>_linux_amd64.tar.gz` |
| Linux em ARM (Raspberry Pi, Graviton) | `procreepy_<version>_linux_arm64.tar.gz` |
| Linux em ARM de 32 bits | `procreepy_<version>_linux_arm.tar.gz` |

Em um Mac, menu Apple → "Sobre este Mac" mostra se você tem Apple Silicon ou
Intel.

### Windows

1. Baixe `procreepy_<version>_windows_amd64.zip`.
2. Clique com o botão direito no arquivo baixado → **Extrair Tudo** → escolha uma
   pasta que você consiga achar depois, por exemplo `Downloads\procreepy`.
3. Abra essa pasta, clique na barra de endereços no topo, digite `cmd` e aperte
   Enter. Abre-se uma janela preta de Prompt de Comando nessa pasta.
4. Coloque um arquivo `.procreate` na mesma pasta e execute:

   ```text
   procreepy.exe "My Artwork.procreate" "My Artwork.mp4"
   ```

   As aspas só importam se o nome tiver espaços.

**Sobre o aviso do SmartScreen.** O programa não é assinado com um certificado
pago da Microsoft, então na primeira execução o Windows pode mostrar uma janela
azul: "O Windows protegeu o seu PC", citando um "aplicativo não reconhecido".
Isso não é um alerta de vírus — o Windows mostra isso para qualquer programa que
ainda não viu com frequência suficiente. Para continuar, clique em
**Mais informações** e depois no botão **Executar assim mesmo**. Se preferir não
fazer isso, use a [imagem de contêiner](#docker--podman).

### macOS

1. Baixe o `.tar.gz` do seu chip (`darwin_arm64` para Apple Silicon,
   `darwin_amd64` para Intel).
2. Abra o Terminal (Aplicativos → Utilitários → Terminal) e vá para a pasta de
   downloads:

   ```bash
   cd ~/Downloads
   ```

3. Descompacte e permita a execução:

   ```bash
   tar -xzf procreepy_*_darwin_*.tar.gz
   xattr -d com.apple.quarantine ./procreepy
   ```

   A linha `xattr` remove a marca de quarentena dos downloads. Sem ela o macOS se
   recusa a iniciar o programa, porque ele não é notarizado pela Apple.

4. Converta um arquivo:

   ```bash
   ./procreepy "My Artwork.procreate" "My Artwork.mp4"
   ```

Para poder digitar `procreepy` de qualquer lugar, mova-o para o PATH:
`sudo mv ./procreepy /usr/local/bin/`.

### Linux

```bash
tar -xzf procreepy_*_linux_amd64.tar.gz
./procreepy artwork.procreate artwork.mp4
```

O programa é ligado estaticamente, então roda em qualquer distribuição
independentemente da versão do glibc. Para instalar para todos os usuários:
`sudo install -m 755 procreepy /usr/local/bin/`.

### Docker / Podman

Uma imagem de contêiner é publicada em cada release, para `linux/amd64` e
`linux/arm64`. Em um Mac com Apple Silicon a variante arm64 é escolhida
automaticamente.

```bash
# Docker
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4

# Podman (rootless, Linux): mapeie seu usuário e reetiquete a montagem com :Z
podman run --rm --userns=keep-id --user "$(id -u):$(id -g)" \
  -v "$PWD":/data:Z -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4
```

Os dois motores não são intercambiáveis opção por opção: o Podman rootless
exige `--userns=keep-id`, `--user` e — em um host com SELinux — uma montagem com
`:Z`, ou a execução falha por permissão no diretório de saída. Para fixar uma
versão use `:0.3.0` em vez de `:latest` (as tags da imagem não levam o prefixo
`v`). Detalhes sobre a propriedade dos arquivos, SELinux e o resto em
[usage.md](usage.md#container-usage).

### Compilar a partir do código

Necessário apenas se você quiser mudar o código. Veja
[development.md](development.md).

### Verificar o download (opcional)

Cada release também publica `CHECKSUMS.txt`. Para confirmar que o download chegou
íntegro, mostre o hash do seu arquivo e compare com a linha correspondente:

```bash
sha256sum procreepy_0.3.0_linux_amd64.tar.gz    # Linux
shasum -a 256 procreepy_0.3.0_darwin_arm64.tar.gz   # macOS
grep darwin_arm64 CHECKSUMS.txt                 # o valor esperado
```

No Windows: `certutil -hashfile procreepy_0.3.0_windows_amd64.zip SHA256`.

Os dois valores devem ser idênticos. É um passo opcional — ele detecta um
download truncado ou adulterado, nada além disso.

## Converter um arquivo

```bash
procreepy artwork.procreate artwork.mp4
```

Resultado:

```text
artwork.mp4
```

O original `artwork.procreate` não é modificado. Se `artwork.mp4` já existir, ele
é substituído, mas só depois que o novo vídeo for escrito por completo.

Você também pode indicar uma pasta como destino e deixar o procreepy nomear o
arquivo:

```bash
procreepy artwork.procreate videos/
```

Resultado: `videos/artwork.mp4`. A pasta precisa existir antes.

## Converter uma pasta

```bash
procreepy input/
```

Lê todos os `.procreate` em `input/` e escreve em `input_procreepy/`, como
mostrado em [O que você obtém](#o-que-você-obtém). Para escolher o destino você
mesmo:

```bash
procreepy input/ ~/Videos/timelapses
```

Para incluir subpastas (a estrutura delas é espelhada na saída):

```bash
procreepy -r input/ output/
```

O que acontece durante a execução:

- O progresso é informado por arquivo, uma linha para cada.
- Um arquivo cujo timelapse nunca foi gravado ainda produz seu projeto enxuto (e
  seu PSD com `--psd`) — só o vídeo é ignorado, com uma nota, e a execução
  continua.
- Um arquivo danificado é relatado como erro, a execução segue com os demais, e o
  comando termina com código de saída `1` para que scripts percebam.
- Executar o mesmo comando duas vezes não refaz o trabalho pronto: artes cujos
  resultados já estão lá são ignoradas. Acrescente `-f` para refazê-las.
- Após uma execução sem falhas, os projetos enxutos são empacotados em
  `procreate.zip` e a pasta `procreate/` é removida. Uma execução com qualquer
  falha mantém a pasta sem empacotar, para que você possa inspecioná-la e
  retomar. Use `--no-zip` para manter sempre a pasta.

## Verificar um arquivo antes de converter

Nenhum destes comandos cria vídeo nem muda nada.

**Existe um timelapse neste arquivo, e de que tamanho?**

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

`--list` lê o índice do arquivo. É instantâneo e diz se existe um timelapse.

**A conversão vai realmente funcionar?**

```bash
procreepy --verify artwork.procreate
```

`--verify` vai mais longe: lê cada segmento do timelapse, procura danos e
confirma que os segmentos podem ser unidos sem recodificar. Mais lento que
`--list`, e a resposta honesta para "isto vai converter direito?".

Ambos também aceitam uma pasta, e então relatam cada arquivo dentro dela.

## Exportar para PSD

```bash
procreepy --psd input/ output/
```

Ao lado de cada vídeo e projeto enxuto, é escrito `output/psd/NAME.psd`: um
arquivo do Photoshop em camadas que abre no Photoshop, Affinity Photo, GIMP e
similares.

`--psd` funciona **somente com uma pasta de entrada**. Com um arquivo único, o
comando para com `--psd needs a directory INPUT; it writes into OUTPUT/psd/`.

O PSD é uma exportação, não uma cópia perfeita. Ele mantém a árvore de camadas, a
estrutura e a ordem dos grupos, os nomes, a visibilidade, a opacidade, os modos
de mesclagem e a própria imagem; **não** mantém máscaras de camada, relações de
recorte nem texto editável. Leia
[o que o PSD preserva e o que ele perde](usage.md#export-a-psd) antes de usá-lo
para trabalho final. Mantenha o arquivo `.procreate` como cópia mestra.

O PSD também embute uma pequena imagem de prévia, de modo que apps que o leem
(Photoshop, Affinity, GIMP) mostram uma miniatura. Isso **não** faz, por si só, o
Explorador do Windows desenhar uma miniatura: ele precisa de um manipulador de
miniaturas registrado para `.psd`, que o Windows não traz (o Photoshop ou um
pacote como o SageThumbs fornece um), e para `.procreate` não existe nenhum.

## O que acontece com seus arquivos

- **Seus originais nunca são modificados.** O procreepy abre os `.procreate`
  somente para leitura. Tudo o que ele produz é escrito em outro lugar.
- **Nada fica escrito pela metade.** Cada resultado é construído primeiro em um
  arquivo temporário e colocado no lugar apenas quando está completo. Uma
  execução interrompida ou com falha nunca deixa um vídeo quebrado nem danifica
  um arquivo que já existia.
- **Execuções em pasta publicam por arte, como um conjunto.** O vídeo, o projeto
  enxuto e o PSD de uma arte aparecem juntos ou não aparecem — você nunca fica
  com um vídeo sem o projeto dele.
- **As datas são preservadas.** Cada saída — o vídeo, o projeto enxuto e o PSD —
  recebe a data de modificação do `.procreate` de que veio (e, no Windows, também
  a de criação), de modo que um projeto reimportado no Procreate mantém seu lugar
  na galeria.
- **Executar de novo é seguro.** Artes concluídas são ignoradas, quer os projetos
  ainda sejam uma pasta, quer já estejam empacotados em `procreate.zip`. Um
  conjunto que ficou incompleto por uma execução interrompida é refeito por
  inteiro. `-f` refaz tudo.
- **Converter um arquivo substitui o destino** se ele existir, depois que o novo
  vídeo é escrito por completo.
- **Arquivos grandes precisam de espaço temporário.** Timelapses grandes são
  montados através de um arquivo temporário. Se o espaço acabar, aponte
  `--tmpdir` para um disco mais folgado.

## Se algo der errado

| O que você vê | O que significa |
|---|---|
| `procreepy: command not found` | Você não está na pasta onde descompactou; no macOS/Linux use `./procreepy`. |
| `no video/segments in the archive` | Nenhum timelapse foi gravado nesse arquivo. Não há como recuperá-lo. |
| `input is not a valid ZIP archive` | Não é um arquivo `.procreate`, ou o download/cópia está truncado. |
| `segment ... is corrupted inside the archive` | Os dados do timelapse estão danificados. |
| `segments are incompatible` | O timelapse foi gravado atravessando uma troca de tela ou de qualidade, então não dá para unir sem recodificar. |
| `refusing to write video data to a terminal` | Informe um nome de arquivo de destino, ou redirecione com `> out.mp4`. |
| `O Windows protegeu o seu PC` | Veja [a nota sobre o SmartScreen](#windows). |
| `no space left` / erros de escrita | Use `--tmpdir` em um disco com mais espaço livre. |

Cada um desses casos, com o sintoma exato e o que fazer, está em
[troubleshooting.md](troubleshooting.md).

## Referência de comandos

```text
procreepy [options] INPUT [OUTPUT]
```

`INPUT` é um arquivo `.procreate`, uma pasta com eles, ou `-` para a entrada
padrão. `OUTPUT` é um nome de arquivo, uma pasta, ou `-` para a saída padrão.
Para um arquivo único, omitir `OUTPUT` escreve o vídeo na saída padrão; para uma
pasta, o padrão é `<INPUT>_procreepy/` no diretório atual.

| Opção | O que faz | Aplica-se a |
|---|---|---|
| `-h`, `--help` | mostrar a ajuda e sair | sempre |
| `--version` | mostrar a versão e sair | sempre |
| `--list` | listar os segmentos do timelapse; não escrever vídeo | arquivo ou pasta |
| `--verify` | verificar cada segmento; não escrever vídeo | arquivo ou pasta |
| `-r`, `--recursive` | processar também as subpastas | só entrada de pasta |
| `-f`, `--force` | sobrescrever resultados que já existem | só entrada de pasta |
| `--psd` | exportar também um PSD em camadas por arte | só entrada de pasta |
| `--no-zip` | deixar os projetos como pasta `procreate/` em vez de empacotar `procreate.zip` | só entrada de pasta |
| `--strict` | tratar falhas na numeração de segmentos como erros, não avisos | arquivo ou pasta |
| `--tmpdir DIR` | onde colocar os arquivos temporários | sempre |
| `-q`, `--quiet` | imprimir apenas avisos e erros | sempre |
| `--` | parar de ler opções; tratar o resto como nomes de arquivo | sempre |

Referência completa com exemplos, formatos de saída e códigos de saída:
[usage.md](usage.md).

## Como funciona

Um arquivo `.procreate` é um arquivo ZIP. Quando a gravação de timelapse está
ligada, o Procreate guarda o vídeo finalizado dentro dele, dividido em pedaços
numerados (`video/segments/segment-1.mp4`, `segment-2.mp4`, …). O procreepy lê
esses pedaços direto do arquivo, ordena-os numericamente, confere que compartilham
codec e tela, e os costura em um único MP4 copiando os quadros sem tocá-los. Nada
é renderizado e nada é recodificado — por isso é rápido e sem perdas.

O caminho do timelapse nunca olha as suas camadas. Só `--psd` lê a arte em si.

Detalhes — montagem do MP4, escrita atômica, estratégia de arquivos temporários,
fidelidade do PSD: [how-it-works.md](how-it-works.md).

## Desenvolvimento

Compilação, testes, cobertura, compilação cruzada, release e CI:
[development.md](development.md).

Os requisitos são Go (a versão está em [`go.mod`](../go.mod)) e `make`. O projeto
não tem dependências de terceiros.

```bash
make check   # verificação de formato + build + vet + toda a suíte de testes
```

## Licença

Apache License 2.0 — veja [LICENSE](../LICENSE).

## Idiomas

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
