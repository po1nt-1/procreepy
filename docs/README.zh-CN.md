# procreepy

一个小型跨平台工具（Linux、Windows、macOS）：从 `.procreate` 文件中提取现成的归档延时摄影，并将其片段合并成一个 MP4。不会重新编码（stream copy），也不会进行渲染。

`.procreate` 是一个 ZIP 压缩包。如果启用了延时摄影录制，其中会包含：

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
工具只处理这些文件：按**数字顺序**排序（`segment-9` 在 `segment-10` 之前），解析每个片段的 MP4 结构，并将它们重新组装成一个 moov-first MP4，视频帧原样复制。它绝不会打开 `Document.archive`、图层或栅格数据块（`*.lz4`）。

## 要求

没有外部依赖：不需要 `ffmpeg` 或 `ffprobe`。构建只需要 Go（版本见 `go.mod`）和 `make`（所有支持的平台都提供；在精简系统上可通过包管理器安装）。
Makefile 是规范的构建入口：它固定了与 CI 相同的封闭构建环境（离线模块模式、本地 toolchain、无 cgo），并会自动定位 Go toolchain。

```bash
make build      # 编译全部内容并生成可运行的 ./procreepy
make check      # gofmt + build + vet + 完整测试套件
```

`make build` 会在二进制文件中写入 `dev-<commit>`，因此 `procreepy --version` 可以显示它来自哪个 commit（release tarball 则记录对应的 tag）。不使用 `make` 时，等价命令是 `go build ./... && go build -o procreepy ./cmd/procreepy`（如果在 Git checkout 中构建，该二进制会报告 `dev` 或 `dev-<commit>`）。

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
所有构建都是静态的（无 cgo）：Linux 二进制文件可以在任意发行版上运行，与其 glibc 版本无关。CI 流水线（GitLab 和 GitHub）会在每次 commit 时构建这些目标；`dist` 任务发布 tarball 和 `SHA256SUMS` 清单，`repro:*` 任务证明二进制文件可以逐位复现。

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
procreepy --split artwork.procreate artwork.mp4
```
支持全部四种 `INPUT`/`OUTPUT` 组合：`FILE OUTPUT`、`FILE -`、`- OUTPUT`、`- -`。如果省略 `OUTPUT`，则写入 stdout。如果 `OUTPUT` 是一个已存在的目录，视频会使用原始名称写入该目录（`procreepy art.procreate videos/` → `videos/art.mp4`）。

### 批量模式：一个 `.procreate` 文件夹 → 一个视频文件夹

场景：“`input/` 中有很多 `.procreate` 文件，希望视频放到 `output/timelaps/` 中”：

```bash
procreepy input/
```

```text
input/                              output/timelaps/
├── Portrait of a Cat.procreate →   ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →   ├── Landscape v2.mp4
└── No Timelapse.procreate              └──（跳过，并给出警告）
```
- **命名**：`<去掉 .procreate 的原始名称>.mp4`。空格、西里尔字母和特殊字符均原样保留。
- **输出目录**：默认是相对于当前目录的 `output/timelaps/`，并自动创建。也可以作为第二个参数指定其他目录：`procreepy input/ ~/Videos/procreate`。
- **重复运行是安全的**：已经存在的视频会被跳过。要全部重新生成，请使用 `--force`（`-f`）。
- **`-r`** 也会进入子目录；结果中会镜像子目录结构（`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`），因此不同目录中的同名文件不会冲突。
- **单个损坏文件不会阻止其余文件处理。** 没有延时摄影的文件（录制未开启）是警告而不是错误。损坏文件属于错误：会出现在最终汇总中，退出码变为 `1`。
- 隐藏文件（`._Foo.procreate`，macOS 在复制时可能留下）会被忽略。
- 原始文件绝不会被修改。
示例输出（全部写入 stderr）：

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```
`--list` 和 `--verify` 也接受目录，并会遍历其中的所有文件。

### 拆分：视频 + 精简项目（`--split`）

原因是：延时摄影占用的空间可能比绘图本身还大——例如备份到 iPad 时，可能希望将两者分开保存。`--split` 会在每个完成的 `MP4` 旁写出一份精简的项目副本，**不包含** `video/` 下的任何内容：

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                   （延时摄影，无损）
                     →  artwork.procreepy.procreate （同一个项目，不含 video/）
```
批量模式同样适用：每个 `X.mp4` 旁都会出现 `X.procreepy.procreate`。归档中的其他成员（图层、`Info.plist`、预览图）都会逐字节保留：顺序、压缩方式和时间戳都不变。原始 `.procreate` 不会被修改；精简副本不能写入 stdout，因此 `--split` 要求 `OUTPUT` 是一个文件。

### 诊断

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
工具会直接从归档中解析每个片段（包括 ZIP 内部的 CRC 校验），逐行输出报告，并检查这些片段是否可以在不重新编码的情况下合并。**不会创建输出视频。**`--list` 只读取 ZIP 目录。

## 选项

| 选项 | 作用 |
|---|---|
| `-r`, `--recursive` | 目录输入：同时进入子目录 |
| `-f`, `--force` | 目录输入：覆盖已经存在的视频 |
| `--strict` | 将缺失的片段编号视为错误（默认只是警告） |
| `--reencode` | 为兼容旧脚本而接受；不会重新编码，始终使用 stream copy |
| `--split` | 在每个 `MP4` 旁写入 `X.procreepy.procreate`——不含 `video/` 的项目 |
| `--tmpdir DIR` | 临时文件的存放位置 |
| `-q`, `--quiet` | 只输出警告和错误 |

## 工作原理

1. `INPUT` 可以是文件或 stdin。stdin（以及任何不可随机访问的输入）会先写入临时文件，因为 ZIP 需要随机访问。
2. 校验 ZIP（只读），并定位 `video/segments/segment-N.mp4` 条目。
3. 按数字排序。编号出现间隔时给出警告；没有数字的名称会在警告后忽略。
4. 直接从 ZIP 中解析每个片段（不做完整解压）：MP4 boxes、轨道大小、编解码器参数。发现第一个损坏后立即停止。
5. 检查兼容性（分辨率、编解码器、SPS/PPS 集合、音频）。否则 `-c copy` 可能静默地产生损坏结果，因此不兼容会作为错误并给出明确消息，而不是让问题出现在最终视频里。
6. 组装 moov-first MP4：`ftyp`、`moov`（所有轨道，从各片段中截取），然后按播放顺序依次写入 `mdat`。
7. 临时文件（如果有）始终删除：成功、出错、Ctrl+C 或 SIGTERM 时都如此。

### 写入文件和 stdout

两种输出路径都会组装相同的 moov-first MP4：先写入 moov atom，因为帧直接从源片段复制，而且在开始写入前元数据已经已知。写入文件时，这是适合播放器和编辑器的“经典”MP4；写入管道时得到的是完全相同的文件——`> artwork.mp4` 与显式执行 `procreepy artwork.procreate artwork.mp4` 结果一致。
- **写入文件**是原子的：目标旁边先创建 `.partial` 文件，只有成功后才重命名。失败的运行不会留下残文件，也绝不会破坏已有文件。
- stdout 从不混入文本。所有日志行（`level=INFO`/`WARN`/`ERROR`，每行一个结构化的 key=value 记录）都写入 stderr。唯一例外是 `--list`/`--verify` 报告，此时 stdout *就是*结果。如果 stdout 是终端，工具会拒绝向其中输出二进制 MP4。

### 临时文件和 Fedora

在 Fedora 上，`/tmp` 是位于内存中的 tmpfs。延时摄影片段可能达到数百 MB，从 stdin 读取时，整个 `.procreate` 都需要先写入临时文件。因此临时目录按以下顺序选择：`--tmpdir` → `$TMPDIR` → `/var/tmp`（磁盘）→ 系统默认值。如果在临时写入期间磁盘空间耗尽，会得到带提示的明确错误（使用 `--tmpdir` 指向更大的磁盘目录），而不是简单的“磁盘空间不足”。

## 退出码

| 代码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 意外错误；批量模式下——至少有一个文件失败 |
| 2 | 参数错误；输出会覆盖输入；stdout 是终端 |
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

它不会解析 `Document.archive`（NSKeyedArchive），不会触碰 `*.lz4`，不会恢复图层，也不会渲染图像。如果文件中没有录制延时摄影，工具无法从绘画历史中恢复它。注意：对从 `.procreate` 中取出的 `.lz4` 文件执行 `lz4 -t` 并不能进行完整性检查，因为这些文件并不是独立的 LZ4 帧。

## 格式参考

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## 许可证

Apache License 2.0，见 `LICENSE`。

---

## 语言

[English](README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
