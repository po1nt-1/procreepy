# procreepy

把 Procreate 早已录制在 `.procreate` 文件里的延时视频，取出来变成一个普通的 MP4。

- 不重新编码：直接复制帧，画质与 Procreate 原始录制一致。
- 你的 `.procreate` 原文件永不被修改。
- 一次处理一个文件，或者一整个文件夹。
- 只有一个程序文件。不需要 ffmpeg、不需要 Python、不需要账号、无需配置。
- 在 Windows、macOS 和 Linux 上离线运行。

## 这个工具适合你吗

适合使用 procreepy，如果：

- 你有 `.procreate` 文件；
- 绘制时**开启了延时录制**（Procreate 默认开启）；
- 你想把这段延时视频导出成可上传、可剪辑、可保存的 MP4；
- 或者你想从作品中清除延时数据，让文件变小。

procreepy **无法**：

- 生成一段从未录制过的延时视频——它只能提取已经存在的；
- 从图层或撤销历史重建延时视频；
- 修复损坏的 `.procreate` 文件；
- 导出作品的成品图像（但可以导出分层 PSD，见[导出 PSD](#导出-psd)）。

不确定文件里有没有延时视频？[先检查一下](#转换前先检查文件)——一条命令，什么都不会生成。

## 你会得到什么

一个文件进，一个视频出：

```text
my-art.procreate  →  procreepy  →  my-art.mp4
```

一个文件夹进，两个文件夹出：

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

- `timelapses/` 里是视频。
- `projects/` 里是每件作品**移除延时数据后**的副本：体积小得多，并且可以重新导入
  Procreate。项目中其余内容逐字节保留。
- `input/` 保持原样，分毫不动。

## 安装

从发布页面下载适合你系统的现成程序——不需要 Go，也不需要任何构建工具。

- GitLab：https://gitlab.com/po1nt-1/procreepy/-/releases
- GitHub：https://github.com/po1nt-1/procreepy/releases

选择与你电脑匹配的文件：

| 你的系统 | 下载 |
|---|---|
| Windows（大多数 PC） | `procreepy_<version>_windows_amd64.zip` |
| ARM 架构的 Windows | `procreepy_<version>_windows_arm64.zip` |
| Apple Silicon 的 Mac（M1–M5） | `procreepy_<version>_darwin_arm64.tar.gz` |
| Intel 的 Mac | `procreepy_<version>_darwin_amd64.tar.gz` |
| Linux（大多数 PC） | `procreepy_<version>_linux_amd64.tar.gz` |
| ARM 架构的 Linux（树莓派、Graviton） | `procreepy_<version>_linux_arm64.tar.gz` |
| 32 位 ARM 的 Linux | `procreepy_<version>_linux_arm.tar.gz` |

在 Mac 上，点苹果菜单 →「关于本机」即可看到你用的是 Apple Silicon 还是 Intel。

### Windows

1. 下载 `procreepy_<version>_windows_amd64.zip`。
2. 右键点击下载的文件 →**全部解压缩**→选一个以后能找到的文件夹，例如
   `Downloads\procreepy`。
3. 打开该文件夹，点击顶部的地址栏，输入 `cmd` 并回车。会在该文件夹中打开一个黑色的
   命令提示符窗口。
4. 把一个 `.procreate` 文件放进同一个文件夹，然后运行：

   ```text
   procreepy.exe "My Artwork.procreate" "My Artwork.mp4"
   ```

   只有名称中含空格时才需要引号。

**关于 SmartScreen 警告。** 这个程序没有用付费的微软证书签名，所以首次运行时
Windows 可能弹出蓝色窗口「Windows 已保护你的电脑」，并称其为「无法识别的应用」。
这不是病毒报告——任何 Windows 尚未见得足够多的程序都会触发它。要继续，请点击
**更多信息**，再点出现的**仍要运行**按钮。如果你不愿意这样做，可以改用
[容器镜像](#docker--podman)。

### macOS

1. 下载对应芯片的 `.tar.gz`（Apple Silicon 用 `darwin_arm64`，Intel 用
   `darwin_amd64`）。
2. 打开「终端」（应用程序 → 实用工具 → 终端），进入下载文件夹：

   ```bash
   cd ~/Downloads
   ```

3. 解压并允许运行：

   ```bash
   tar -xzf procreepy_*_darwin_*.tar.gz
   xattr -d com.apple.quarantine ./procreepy
   ```

   `xattr` 这一行移除下载隔离标记。没有它，macOS 会拒绝启动该程序，因为它未经
   Apple 公证。

4. 转换一个文件：

   ```bash
   ./procreepy "My Artwork.procreate" "My Artwork.mp4"
   ```

想在任何目录下直接输入 `procreepy`，把它移到 PATH 里：
`sudo mv ./procreepy /usr/local/bin/`。

### Linux

```bash
tar -xzf procreepy_*_linux_amd64.tar.gz
./procreepy artwork.procreate artwork.mp4
```

程序是静态链接的，因此在任何发行版上都能运行，与其 glibc 版本无关。为所有用户
安装：`sudo install -m 755 procreepy /usr/local/bin/`。

### Docker / Podman

每个发布版本都会推送容器镜像，覆盖 `linux/amd64` 和 `linux/arm64`。在 Apple
Silicon 的 Mac 上会自动选中 arm64 变体。

```bash
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4
```

把 `docker` 原样换成 `podman` 即可。要固定版本，用 `:0.3.0` 代替 `:latest`（镜像
标签不带 `v` 前缀）。文件归属等细节见 [usage.md](usage.md#container-usage)。

### 从源码构建

只有想改代码时才需要。见 [development.md](development.md)。

### 校验下载（可选）

每个发布版本还会附带 `CHECKSUMS.txt`。要确认下载完整，输出你文件的哈希，并与其中
对应的那一行比对：

```bash
sha256sum procreepy_0.3.0_linux_amd64.tar.gz    # Linux
shasum -a 256 procreepy_0.3.0_darwin_arm64.tar.gz   # macOS
grep darwin_arm64 CHECKSUMS.txt                 # 期望值
```

Windows 上：`certutil -hashfile procreepy_0.3.0_windows_amd64.zip SHA256`。

两个值必须完全一致。这一步可选——它能发现被截断或被篡改的下载，仅此而已。

## 转换单个文件

```bash
procreepy artwork.procreate artwork.mp4
```

结果：

```text
artwork.mp4
```

原文件 `artwork.procreate` 不会被修改。如果 `artwork.mp4` 已存在，它会被替换，
但只在新视频完整写入之后。

你也可以给一个文件夹作为目标，让 procreepy 自己命名：

```bash
procreepy artwork.procreate videos/
```

结果：`videos/artwork.mp4`。该文件夹必须已经存在。

## 转换整个文件夹

```bash
procreepy input/
```

读取 `input/` 中的每个 `.procreate`，写入 `output/`，结构如
[你会得到什么](#你会得到什么)所示。要自己指定目标：

```bash
procreepy input/ ~/Videos/timelapses
```

要包含子文件夹（其结构会在输出中镜像保留）：

```bash
procreepy -r input/ output/
```

运行过程中会发生什么：

- 进度按文件逐行报告。
- 从未录制延时的文件会**带警告跳过**。不会为它写任何东西，运行继续。
- 损坏的文件会报为错误，运行仍会继续处理其余文件，命令最终以退出码 `1` 结束，
  便于脚本察觉。
- 同一条命令跑第二次不会重做已完成的工作：结果已在的作品会被跳过。加上 `-f`
  可强制重建。

## 转换前先检查文件

这两条命令都不生成视频，也不改动任何东西。

**这个文件里有延时视频吗，有多长？**

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

`--list` 只读取文件的目录表。瞬间完成，用来回答「里面到底有没有延时视频」。

**转换真的能成功吗？**

```bash
procreepy --verify artwork.procreate
```

`--verify` 更进一步：读取每个延时片段、检查是否损坏，并确认这些片段能够在不重新
编码的前提下拼接起来。比 `--list` 慢，但它才是「这个能干净地转换出来吗」的诚实
答案。

两者也都接受文件夹，此时会对其中每个文件给出报告。

## 导出 PSD

```bash
procreepy --psd input/ output/
```

在每个视频和精简项目旁边，会写出 `output/psd/NAME.psd`：一个分层的 Photoshop
文件，可用 Photoshop、Affinity Photo、GIMP 等打开。

`--psd` **只支持文件夹作为输入**。若给单个文件，命令会停下并提示
`--psd needs a directory INPUT; it writes into OUTPUT/psd/`。

PSD 是一次导出，而不是完美副本。它保留图层树与图层名、可见性、不透明度、混合模式
以及图像本身；**不**保留图层蒙版、剪贴关系和可编辑文字。在把它用于正式产出之前，
请先阅读 [PSD 保留了什么、丢失了什么](usage.md#export-a-psd)。请始终把
`.procreate` 文件当作母版。

## 你的文件会怎样

- **你的原文件永不被修改。** procreepy 以只读方式打开 `.procreate`。它产出的一切
  都写在别处。
- **不会留下写了一半的东西。** 每个结果先在临时文件中生成，只有完整后才被放到位。
  被中断或失败的运行绝不会留下损坏的视频，也不会破坏原本已存在的文件。
- **文件夹模式按作品成套发布。** 一件作品的视频、精简项目和 PSD 要么一起出现，
  要么都不出现——你绝不会得到一个没有对应项目的视频。
- **重复运行是安全的。** 已完成的作品会被跳过。因中断而残缺的一套会被整体重建。
  `-f` 则重建全部。
- **转换单个文件会替换目标文件**（如果存在），且在新视频完整写入之后才替换。
- **大文件需要临时空间。** 大型延时视频通过临时文件组装。空间不足时，用
  `--tmpdir` 指向更宽裕的磁盘。

## 如果出错了

| 你看到的 | 含义 |
|---|---|
| `procreepy: command not found` | 你不在解压所在的文件夹；macOS/Linux 上请用 `./procreepy`。 |
| `no video/segments in the archive` | 该文件没有录制延时视频，无法恢复。 |
| `input is not a valid ZIP archive` | 不是 `.procreate` 文件，或下载/拷贝被截断。 |
| `segment ... is corrupted inside the archive` | 延时数据已损坏。 |
| `segments are incompatible` | 延时录制跨越了画布或画质切换，无法在不重新编码的情况下拼接。 |
| `refusing to write video data to a terminal` | 请给出目标文件名，或用 `> out.mp4` 重定向。 |
| `Windows 已保护你的电脑` | 见 [SmartScreen 说明](#windows)。 |
| `no space left` / 写入错误 | 用 `--tmpdir` 指向可用空间更多的磁盘。 |

以上每一种情况的确切症状与处理办法，都在
[troubleshooting.md](troubleshooting.md)。

## 命令参考

```text
procreepy [options] INPUT [OUTPUT]
```

`INPUT` 是一个 `.procreate` 文件、一个装着它们的文件夹，或 `-` 表示标准输入。
`OUTPUT` 是文件名、文件夹，或 `-` 表示标准输出。对单个文件，省略 `OUTPUT` 表示把
视频写到标准输出；对文件夹，默认是 `output/`。

| 选项 | 作用 | 适用于 |
|---|---|---|
| `-h`、`--help` | 显示帮助并退出 | 始终 |
| `--version` | 显示版本并退出 | 始终 |
| `--list` | 列出延时片段；不写视频 | 文件或文件夹 |
| `--verify` | 检查每个片段；不写视频 | 文件或文件夹 |
| `-r`、`--recursive` | 同时处理子文件夹 | 仅文件夹输入 |
| `-f`、`--force` | 覆盖已存在的结果 | 仅文件夹输入 |
| `--psd` | 为每件作品额外导出分层 PSD | 仅文件夹输入 |
| `--strict` | 把片段编号的缺口视为错误而非警告 | 文件或文件夹 |
| `--tmpdir DIR` | 临时文件放在哪里 | 始终 |
| `-q`、`--quiet` | 只打印警告和错误 | 始终 |
| `--` | 停止解析选项；其余一律当作文件名 | 始终 |

含示例、输出格式和退出码的完整参考：[usage.md](usage.md)。

## 工作原理

`.procreate` 文件就是一个 ZIP 压缩包。开启延时录制时，Procreate 会把成品视频存在
里面，并切成带编号的若干片段（`video/segments/segment-1.mp4`、`segment-2.mp4`
……）。procreepy 直接从压缩包里读取这些片段，按数字排序，确认它们的编解码器与画布
一致，然后原样复制帧并缝成一个 MP4。全程不渲染、不重新编码——所以又快又无损。

延时提取这条路径从不查看你的图层。只有 `--psd` 会读取作品本身。

更多细节——MP4 组装、原子写入、临时文件策略、PSD 保真度：
[how-it-works.md](how-it-works.md)。

## 开发

构建、测试、覆盖率、交叉编译、发布与 CI：[development.md](development.md)。

前置条件是 Go（版本见 [`go.mod`](../go.mod)）和 `make`。本项目没有任何第三方依赖。

```bash
make check   # 格式检查 + 构建 + vet + 完整测试套件
```

## 许可证

Apache License 2.0 — 见 [LICENSE](../LICENSE)。

## 语言

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
