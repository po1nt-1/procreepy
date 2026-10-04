# procreepy

一个小型跨平台工具（Linux、Windows、macOS）：从 `.procreate` 文件中提取现成的归档延时摄影，并将其片段合并成一个 MP4。不会重新编码（stream copy），也不会进行渲染。

`.procreate` 是一个 ZIP 压缩包。如果启用了延时摄影录制，其中会包含：

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
工具只处理这些文件：按**数字顺序**排序（`segment-9` 在 `segment-10` 之前），解析每个片段的 MP4 结构，并将它们重新组装成一个 moov-first MP4，视频帧原样复制。在批量模式下，它为每件处理过的作品额外写出一个不含延时摄影、可再次导入的项目副本，加 `--psd` 时再写一份带图层的 Photoshop PSD 文档。延时摄影这条路径本身绝不打开 `Document.archive`、图层或栅格数据块（`*.lz4`）——只有 `--psd` 会打开。

## 要求

没有外部依赖：不需要 `ffmpeg` 或 `ffprobe`。构建只需要 Go（版本见 `go.mod`）和 `make`（所有支持的平台都提供；在精简系统上可通过包管理器安装）。
Makefile 是规范的构建入口：它固定了与 CI 相同的封闭构建环境（离线模块模式、本地 toolchain、无 cgo），并会自动定位 Go toolchain。

```bash
make build      # 编译全部内容并生成可运行的 ./procreepy
make check      # gofmt + build + vet + 完整测试套件
```

`make build` 会在二进制文件中写入 `dev-<short sha>`（不在 Git checkout 中时为 `dev-nogit`），因此 `procreepy --version` 可以显示它从哪里构建而来（release tarball 则记录对应的 tag）。不使用 `make` 时，等价命令是 `go build ./... && go build -o procreepy ./cmd/procreepy`（该二进制会报告 `dev`，或在可用 VCS 盖章时报告 `dev-<commit>`）。

### 为其他操作系统构建

项目是纯 Go 代码，可以干净地交叉编译到所有支持的目标平台。从任意平台都可以：

| 目标 | 命令 |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM 64 位（Raspberry Pi、Graviton） | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM 32 位 | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64（10/11） | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM 64 位 | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon（M1–M5） | `make release GOOS=darwin GOARCH=arm64` |
每个目标都会生成规范化的 `dist/procreepy-<version>-<os>-<arch>.tar.gz` 并打印其 SHA-256；`make cross` 会一次构建完整矩阵，`make repro` 则证明构建可以逐位复现。单个目标的直接等价命令是 `GOOS=… GOARCH=… go build -o procreepy[.exe] ./cmd/procreepy`。
所有构建都是静态的（无 cgo）：Linux 二进制文件可以在任意发行版上运行，与其 glibc 版本无关。CI 流水线（GitLab 和 GitHub）会在每次 commit 时构建这些目标，并且由于 Windows 是主要平台，还会原生地在 Windows 上运行完整测试套件；`dist` 任务发布 tarball 和 `SHA256SUMS` 清单，`repro:*` 任务证明二进制文件可以逐位复现。

- Linux/macOS：无需安装，直接运行二进制文件。
- Windows：二进制文件未签名，因此 SmartScreen 可能显示“已保护你的电脑”——选择 **更多信息 → 仍要运行**。

## 用法

单个文件：

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
```
支持全部四种 `INPUT`/`OUTPUT` 组合：`FILE OUTPUT`、`FILE -`、`- OUTPUT`、`- -`。如果省略 `OUTPUT`，则写入 stdout。如果 `OUTPUT` 是一个已存在的目录，视频会使用原始名称写入该目录（`procreepy art.procreate videos/` → `videos/art.mp4`）。

### 批量模式：一个 `.procreate` 文件夹 → 视频和项目的文件夹

场景：“`input/` 中有很多 `.procreate` 文件，希望结果放到 `output/` 中”：

```bash
procreepy input/
```

每件处理过的作品产出一对——延时摄影和一个不含它、可再次导入的项目：

```text
input/                               output/
├── Portrait of a Cat.procreate  →   ├── timelapses/Portrait of a Cat.mp4
│                                    ├── projects/Portrait of a Cat.procreepy.procreate
├── Landscape v2.procreate       →   ├── timelapses/Landscape v2.mp4
│                                    └── projects/Landscape v2.procreepy.procreate
└── No Timelapse.procreate              （跳过，并给出警告——未写任何东西）
```
- **命名**：`<去掉 .procreate 的原始名称>`，再加 `.mp4` 或
  `.procreepy.procreate`。空格、西里尔字母和特殊字符均原样保留；在不区分
  大小写的文件系统中，仅大小写不同的名称会获得后缀 `-2`、`-3`，……
- **输出目录**：默认是相对于当前目录的 `output/`，并自动创建。也可以作为
  第二个参数指定其他目录：`procreepy input/ ~/Videos/procreate`。
- **精简项目**：`projects/NAME.procreepy.procreate` 是同一个归档，去掉了
  `video/segments/segment-N.mp4` 成员；其余所有成员逐字节搬移（顺序、压缩
  方式和时间戳不变）。项目保留源文件的修改时间，因此重新导入 Procreate 时
  画廊不会被重新排序。
- **重复运行是安全的**：延时摄影和项目都已存在的文件会被跳过；做了一半的
  一对会整体重建。要全部重新生成，请使用 `--force`（`-f`）。
- **`-r`** 也会进入子目录；其结构在两棵树中都有镜像，因此不同目录中的同名
  文件不会冲突。
- **单个损坏文件不会阻止其余文件处理。** 没有延时摄影的文件（录制未开启）
  是警告而不是错误：不为它写任何东西。损坏文件属于错误：会出现在最终汇总
  中，退出码变为 `1`。
- **原子化的一组输出**：一个文件的各项输出（延时摄影 + 项目，加 `--psd` 时
  还有 PSD）先写入临时文件，然后作为一组整体发布——要么全部，要么全无。
  失败的运行绝不会留下一个孤立的视频配上一个缺失的项目。
- 隐藏文件（`._Foo.procreate`，macOS 在复制时可能留下）会被忽略。
- 原始文件绝不会被修改。

示例输出（全部写入 stderr；运行因损坏文件以代码 `1` 结束）：

```text
level=INFO msg="batch conversion started" files=4 input=input/ timelapses=output/timelapses/ projects=output/projects/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate" removed_segments=17 video_size="6.7 MiB"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" timelapse="output/timelapses/Portrait of a Cat.mp4" project="output/projects/Portrait of a Cat.procreepy.procreate" removed_segments=18 video_size="4.1 MiB"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`removed_segments` 是从精简项目中移除的片段文件数，`video_size` 是它们在
归档内的总（压缩后）大小。重复运行同一命令会为每一对已存在的输出来报告：

```text
level=INFO msg="skipped, outputs already exist (use --force to overwrite)" input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate"
```

`--list` 和 `--verify` 也接受目录，并会遍历其中的所有文件。

### PSD 导出（`--psd`）

```bash
procreepy --psd input/ out/
```

仅接受目录输入。每一对转换结果的旁边会写出 `out/psd/NAME.psd`，与其余输出
一起原子化发布：

```text
level=INFO msg="psd exported" input="input/Portrait of a Cat.procreate" psd="out-psd/psd/Portrait of a Cat.psd" layers=3
```

PSD 中包含的内容：

- 图层树（分组、顺序）、图层名称（Unicode）、可见性、不透明度、混合模式、
  边界和锁定状态；
- 每个图层的 8 位 RGBA 像素（用 PackBits 压缩）；
- DPI 和嵌入的 ICC 配置文件；
- 合并后的合成图，逐字取自 Procreate 自身的扁平化渲染结果（当该渲染缺失或
  损坏时，可见图层以 Normal 模式合成作为近似）。

PSD 中不包含的内容——它是导出，而不是无损往返：

- 图层蒙版和精确的“裁剪到下层”语义不会保留；
- 文本图层保留像素，但不可编辑的文本数据不保留；
- 直通 alpha（未预乘）无法精确还原：Procreate 存储的是预乘的 8 位瓦片，
  因此图层边缘的镶边颜色可能有细微差异；
- 宽或高超过 30000 像素的画布会被直接拒绝（PSD 格式的限制，PSB 才行）。

`.procreate` 仍然是母版；把 PSD 当作给 Photoshop 和其他导入工具的快照。

### 诊断

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
工具会直接从归档中解析每个片段（包括 ZIP 内部的 CRC 校验），逐行输出报告，并检查这些片段是否可以在不重新编码的情况下合并。**不会创建输出视频。**`--list` 只读取 ZIP 目录。

## 选项

| 选项 | 作用 |
|---|---|
| `-h`, `--help` | 显示帮助并退出 |
| `--list` | 按播放顺序列出片段并退出 |
| `--verify` | 验证每个片段；不创建输出视频 |
| `-r`, `--recursive` | 目录输入：同时进入子目录 |
| `-f`, `--force` | 目录输入：覆盖已经存在的结果 |
| `--strict` | 将缺失的片段编号视为错误（默认只是警告） |
| `--psd` | 目录输入：为每件作品额外导出带图层的 PSD |
| `--tmpdir DIR` | 临时文件的存放位置（默认：`$TMPDIR`，其次是 `/var/tmp`，再是系统临时目录） |
| `-q`, `--quiet` | 只输出警告和错误 |
| `--version` | 显示版本号并退出 |
| `--` | 停止解析选项；其余按位置参数处理 |

## 工作原理

1. `INPUT` 可以是文件或 stdin。stdin（以及任何不可随机访问的输入）会先写入临时文件，因为 ZIP 需要随机访问。
2. 校验 ZIP（只读），并定位 `video/segments/segment-N.mp4` 条目。
3. 按数字排序。编号出现间隔时给出警告；没有数字的名称会在警告后忽略。
4. 直接从 ZIP 中解析每个片段（不做完整解压）：MP4 boxes、轨道大小、编解码器参数。发现第一个损坏后立即停止。
5. 检查兼容性（分辨率、编解码器、SPS/PPS 集合、音频）。否则 `-c copy` 可能静默地产生损坏结果，因此不兼容会作为错误并给出明确消息，而不是让问题出现在最终视频里。
6. 组装 moov-first MP4：`ftyp`、`moov`（所有轨道，从各片段中截取），然后按播放顺序依次写入 `mdat`。
7. 运行所承诺的一切（MP4、精简项目、PSD）都先进入临时文件，直到最后一项
   成功之后才作为一个整体发布；批量模式下无论如何都会继续下一个输入。
8. 临时文件（如果创建了）始终删除：成功、出错、Ctrl+C 或 SIGTERM 时都如此。

### 写入文件和 stdout

两种输出路径都会组装相同的 moov-first MP4：先写入 moov atom，因为帧直接从源片段复制，而且在开始写入前元数据已经已知。写入文件时，这是适合播放器和编辑器的“经典”MP4；写入管道时得到的是完全相同的文件——`> artwork.mp4` 与显式执行 `procreepy artwork.procreate artwork.mp4` 结果一致。
- **写入文件**是原子的：目标旁边先创建 `.partial` 文件，只有成功后才重命名。失败的运行不会留下残文件，也绝不会破坏已有文件。
- Windows 上的差异：最终的重命名使用带“替换已存在文件”标志的 `MoveFileEx`，因此 `--force` 和重新生成不完整的输出组都会就地替换已有文件，行为与 Unix 一致。它并不像 POSIX rename 那样严格原子；实际的差别只出现在一种情形——如果目标文件还被其他程序占着（例如播放器还开着上一次的 MP4），重命名会被拒绝并报出可读的 `Access is denied` 错误，旧文件保持不动，关闭该程序后重新运行即可成功。
- stdout 从不混入文本。所有日志行（`level=INFO`/`WARN`/`ERROR`，每行一个结构化的 key=value 记录）都写入 stderr。唯一例外是 `--list`/`--verify` 报告，此时 stdout *就是*结果。如果 stdout 是终端，工具会拒绝向其中输出二进制 MP4。

### 控制台颜色

当 stderr 是交互式终端且环境变量 `NO_COLOR` 未设置时，级别令牌 `WARN` 和
`ERROR` 会被着色（黄色和粗体红色）；`INFO` 保持普通。管道、重定向、CI 和
测试都保持逐字节的普通格式，因此不会影响任何脚本。特意不提供 `--color`
选项。

### 临时文件和 Fedora

在 Fedora 上，`/tmp` 是位于内存中的 tmpfs。延时摄影片段可能达到数百 MB，从 stdin 读取时，整个 `.procreate` 都需要先写入临时文件。因此临时目录按以下顺序选择：`--tmpdir` → `$TMPDIR` → `/var/tmp`（磁盘）→ 系统默认值。如果在临时写入期间磁盘空间耗尽，会得到带提示的明确错误（使用 `--tmpdir` 指向更大的磁盘目录），而不是简单的“磁盘空间不足”。

## 退出码

| 代码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 意外错误；批量模式下——至少有一个文件失败 |
| 2 | 参数错误（包括 `--psd` 带单个文件，或把整组结果写入 stdout）；`OUTPUT` 与 `INPUT` 是同一个文件；stdout 是终端 |
| 3 | 输入不存在、为空或不是 ZIP |
| 4 | 归档中没有 `video/segments`（没有录制延时摄影） |
| 5 | 片段损坏；编号存在歧义或缺失（`--strict`） |
| 6 | 保留（未使用：没有外部依赖） |
| 7 | 片段不兼容 stream copy |
| 8 | 保留（未使用：没有外部依赖） |
| 9 | 写入结果或临时文件失败 |
| 130 | 被中断（Ctrl+C / SIGTERM） |

## 测试

```bash
make test          # or: go test ./...
```

不需要真实的 `.procreate` 文件：测试会使用生成的 MP4 片段构建 ZIP（见 `internal/testkit`）。检查是结构性的：解析生成的 MP4、检查 box 顺序、sample 数量和 `mdat` 内容。安装 C 编译器后也可以运行 `-race`，但它不是必需的。
覆盖范围包括：普通文件、缺少 `video/segments`、单个片段、片段顺序错误（`segment-9`/`segment-10`）、stdin、stdout、名称中的空格和特殊字符、损坏和截断的 ZIP、损坏和截断的 MP4、CRC 损坏、写入错误（`/dev/full`）、不兼容片段，以及完整的批量模式。

## 工具明确不会做的事情

延时摄影路径不会解析 `Document.archive`（NSKeyedArchive），不会触碰 `*.lz4`，不会恢复图层，也不会渲染图像；`--psd` 会解析文档——用于上面所述的导出，并遵循该节列出的保真度限制。如果文件中没有录制延时摄影，工具无法从绘画历史中恢复它。注意：对从 `.procreate` 中取出的 `.lz4` 文件执行 `lz4 -t` 并不能进行完整性检查，因为这些文件并不是独立的 LZ4 帧。

## 格式参考

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## 许可证

Apache License 2.0，见 `LICENSE`。

---

## 语言

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
