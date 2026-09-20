# procreepy

一个跨平台的小 Unix 工具（Linux、Windows、macOS）：从 `.procreate` 文件中提
取现成的归档延时摄影（timelapse），并将其各段合并为一个 MP4。不做任何重编码
（stream copy），也不做任何渲染。

`.procreate` 是一个 ZIP 文件。如果录制时开启了延时摄影,里面会有

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

这个工具只取这些文件:按**数字**排序(`segment-9` 排在 `segment-10` 之前),
解析每个段的 MP4 结构,并把它们重组成一个 moov-first 的 MP4,画面原样拷贝。
它完全不会打开 `Document.archive`、图层或栅格块(`*.lz4`)。

## 要求

无任何外部依赖:不需要 `ffmpeg`、`ffprobe`。编译只需要 Go
(版本见 `go.mod`)。

```bash
go build -o procreepy ./cmd/procreepy
```

### 为其他操作系统构建

项目是纯 Go 代码，可以干净地交叉编译到所有支持的目标平台。在任何平台上，
以下任一命令均可使用：

| 目标 | 命令 |
|---|---|
| Linux x86-64 | `GOOS=linux GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM 64 位（树莓派、Graviton） | `GOOS=linux GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM 32 位 | `GOOS=linux GOARCH=arm GOARM=7 go build -o procreepy ./cmd/procreepy` |
| Windows x86-64（10/11） | `GOOS=windows GOARCH=amd64 go build -o procreepy.exe ./cmd/procreepy` |
| Windows ARM 64 位 | `GOOS=windows GOARCH=arm64 go build -o procreepy.exe ./cmd/procreepy` |
| macOS Intel | `GOOS=darwin GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| macOS Apple Silicon（M1–M5） | `GOOS=darwin GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |

所有构建均为静态（无 cgo）：Linux 二进制文件可在任何发行版上运行，无论其
glibc 版本如何。GitLab CI 流水线在每次提交时都会构建上述目标；`dist` 任务
会发布 tarball 和 `SHA256SUMS` 清单，`repro:*` 任务则证明二进制文件可以
逐位复现。

- Linux/macOS：无需安装步骤，直接运行二进制文件。
- Windows：二进制文件未经签名，SmartScreen 可能提示“已保护你的电脑”——请
  选择**更多信息 → 仍要运行**。

## 用法

单个文件:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```

支持全部四种 `INPUT`/`OUTPUT` 组合:`FILE OUTPUT`、`FILE -`、`- OUTPUT`、
`- -`。省略 `OUTPUT` 即写到 stdout。若 `OUTPUT` 是已存在的目录,视频会以
原名放入该目录(`procreepy art.procreate videos/` → `videos/art.mp4`)。

### 批量模式:一个 `.procreate` 目录 → 一个视频目录

场景:`input/` 里有一堆 `.procreate`,希望把视频放进 `output/timelaps/`:

```bash
procreepy input/
```

```text
input/                          output/timelaps/
├── Portrait of a Cat.procreate →  ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →  ├── Landscape v2.mp4
└── No Timelapse.procreate          └── (跳过,给出警告)
```

- **命名**:`<原文件名去掉 .procreate>.mp4`。空格、西里尔字母和特殊字符
  原样保留。
- **输出目录**:默认为相对当前目录的 `output/timelaps/`,自动创建。也可以用
  第二个参数指定别的目录:`procreepy input/ ~/Videos/procreate`。
- **重复运行是安全的**:已存在的视频会被跳过。想全部重建:用 `--force`
  (`-f`)。
- **`-r`** 会进入子目录;子目录结构会在结果中镜像
  (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`),因此不同
  目录下的同名文件不会冲突。
- **一个坏文件不会中断其余文件。** 没有延时摄影的文件(录制未开启)只是
  警告,不是错误。损坏的文件算错误:它会出现在最终汇总里,退出码变为 `1`。
- 隐藏文件(`._Foo.procreate`,macOS 拷贝时留下的)会被忽略。
- 原始文件绝不被修改。

示例输出(全部走 stderr):

```text
info: 4 .procreate file(s) in input -> output/timelaps/
info: [1/4] input/Landscape v2.procreate -> output/timelaps/Landscape v2.mp4
warning: [2/4] input/No Timelapse.procreate: no timelapse video inside, skipped
error: [3/4] input/Corrupt file.procreate: input is not a valid ZIP archive ...
info: [4/4] input/Portrait of a Cat.procreate -> output/timelaps/Portrait of a Cat.mp4
info: summary: 2 converted, 1 without timelapse, 1 FAILED
```

`--list` 和 `--verify` 同样接受目录,并遍历其中所有文件。

### 拆分:视频 + 精简项目(`--split`)

目的:延时摄影占的空间比画稿本身还大——比如备份到 iPad 时,你希望把它们分开
存放。`--split` 会在每个完成的 `MP4` 旁边写一份**去掉** `video/` 下所有
文件的精简项目副本:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (延时摄影,无损)
                     →  artwork.procreepy.procreate  (同一项目,去掉 video/)
```

批量模式下同理:每个 `X.mp4` 旁边都会出现 `X.procreepy.procreate`。档案中
的其余条目(图层、`Info.plist`、预览)逐字节搬运:顺序、压缩方法和时间戳都
保持不变。原始 `.procreate` 不会被修改;精简副本不能写入 stdout,因此
`--split` 要求 `OUTPUT` 是文件。

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

直接从档案中解析每一段(包括 ZIP 内部的 CRC 校验),逐行打印报告,并检查各
段能否不重编码地拼接。**不会生成输出视频。** `--list` 只读 ZIP 目录表。

## 选项

| 选项 | 作用 |
|---|---|
| `-r`, `--recursive` | 目录输入:同时遍历子目录 |
| `-f`, `--force` | 目录输入:覆盖已存在的视频 |
| `--strict` | 将缺失的段编号视为错误(默认是警告) |
| `--reencode` | 仅为兼容旧脚本而接受;没有重编码,永远是 stream copy |
| `--split` | 在每个 `MP4` 旁写出 `X.procreepy.procreate`——去掉 `video/` 的项目 |
| `--tmpdir DIR` | 段解压到哪里 |
| `-q`, `--quiet` | 只打印警告和错误 |

## 工作原理

1. `INPUT` 是文件或 stdin。stdin(以及任何不可寻址输入)会先转储到一个临时
   文件,因为 ZIP 需要随机访问。
2. 校验 ZIP(只读),定位 `video/segments/segment-N.mp4` 条目。
3. 数字排序。编号有缺口是警告;没有编号的名字带警告忽略。
4. 每段直接从 ZIP 中解析(不做完整解压):MP4 box、轨道尺寸、编解码器参数。
   遇到第一处损坏——停止。
5. 兼容性检查(分辨率、codec、SPS/PPS 集合、音频)。否则 `-c copy` 会无声地
   产出垃圾——所以不兼容就是带清晰消息的错误,而不是成品视频里的惊吓。
6. 组装 moov-first MP4:`ftyp`、`moov`(所有轨道,从各段切出),然后按播放
   顺序一段 `mdat` 接一段 `mdat`。
7. 临时文件(若有)总是被删除——无论成功、失败、Ctrl+C 还是 SIGTERM。

### 写入文件与写入 stdout

两条路径组装的是同一个 moov-first MP4:moov atom 先写,因为画面直接拷贝自
源段,元数据在写入开始前就已确定。对文件而言这是"经典"MP4,播放器与编辑器
都能用;发进管道的是完全相同的文件——`> artwork.mp4` 与显式的
`procreepy artwork.procreate artwork.mp4` 结果一致。

- **文件**输出是原子的:目标旁边先写 `.partial`,成功后才改名。失败的运行
  不留残骸,也绝不会破坏已存在的文件。
- stdout 永远不会混入文本。所有 `info:`/`warning:`/`error:` 行都走
  stderr。唯一例外是 `--list`/`--verify` 的报告,那里 stdout *就是* 结果。
  若 stdout 是终端,工具拒绝往里倾倒二进制 MP4。

### 临时文件与 Fedora

在 Fedora 上,`/tmp` 是内存中的 tmpfs。延时摄影的段可能有几百兆字节,而从
stdin 读取时整个 `.procreate` 都要转储。所以临时目录的选择顺序是:
`--tmpdir` → `$TMPDIR` → `/var/tmp`(在磁盘上)。解压前会检查剩余空间;
空间不足时,你会得到一条带提示的清晰错误,而不是工作进行到一半冒出
"No space left"。

## 退出码

| 代码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 意外错误;批量模式下——至少有一个文件失败 |
| 2 | 参数错误;输出会覆盖输入;stdout 是终端 |
| 3 | 输入不存在、为空或不是 ZIP |
| 4 | 档案中没有 `video/segments`(没有录制延时摄影) |
| 5 | 段损坏;编号含糊或缺失(`--strict`) |
| 6 | 保留(未使用:没有外部依赖) |
| 7 | 各段不兼容,无法 stream copy |
| 8 | 保留(未使用:没有外部依赖) |
| 9 | 写结果或临时文件失败 |
| 130 | 被中断(Ctrl+C / SIGTERM) |

## 测试

```bash
go test ./...
```

不需要真实的 `.procreate`:测试用生成的 MP4 段构建 ZIP(见
`internal/testkit`)。检查是结构性的:解析产出的 MP4、box 顺序、采样数、
`mdat` 内容。`-race` 不是必需的,但如果装了 C 编译器就能工作。

已覆盖:普通文件、缺少 `video/segments`、单个段、乱序段
(`segment-9`/`segment-10`)、stdin、stdout、名称中的空格和特殊字符、损坏
和被截断的 ZIP、损坏和被截断的 MP4、CRC 损坏、写入错误(`/dev/full`)、不
兼容的段,以及整个批量模式。

## 工具故意不做的事

不解析 `Document.archive`(NSKeyedArchive),不碰 `*.lz4`,不恢复图层,也不
渲染图像。如果文件里没有录制延时摄影,这个工具无法从绘画历史中把它找回。
注意:对从 `.procreate` 里取出的 `.lz4` 跑 `lz4 -t` 并不是完整性检查——它
们不是独立的 LZ4 帧。

## 格式参考资料

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## 许可证

Apache License 2.0,见 `LICENSE`。

---

## 语言

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
