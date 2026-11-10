# 字镜 · FontLens

看字样，查字体，知覆盖。

字镜是面向 **Windows 11 x64** 的离线字体浏览工具，以 Go 和 MyGo 构建原生界面，通过 DirectWrite 排版、Direct2D 绘制字体预览。

## 功能

- **浏览与查找**：枚举系统及当前用户安装的字体，递归读取自定义文件夹；支持 TTF、OTF、TTC、OTC、WOFF 和 WOFF2，按字体面浏览、全名搜索、格式筛选和自然排序。同一范围内全名相同的字体只显示首次扫描的一份。
- **精确预览与对比**：列表使用字体自身显示固定的 Family Name，详情和对比绑定具体字体文件及集合面；支持自定义文案、12 种语言或字形变体的文学预设、字号、行距、可变字体轴及命名实例，可选择 2–4 款字体并排对比。
- **信息与原始数据**：查看字体属性、字符与字形度量、完整名称记录和表目录，分页读取原始表字节。
- **字符集覆盖**：统计 GB2312、GBK、GB18030-2022、通用规范汉字表、Big5、HKSCS、日韩字表和 Unicode 17.0 区块的覆盖数、百分比与缺字数，也可导入 UTF-8 字表。
- **本地工作流**：支持浅色、深色和跟随系统，恢复来源目录、分栏宽度和预览文案；字体索引与覆盖结果缓存在本地。

## 使用

启动后在左侧选择“全部”或“系统字体”。点击左下角“添加文件夹”递归读取本地字体目录，点击目录树节点限定浏览范围。

选择字体行后，右侧显示详情与覆盖统计；双击行可打开字体文件，右键可复制 Family Name 或打开所在目录。勾选 2–4 行后点击“对比”，使用同一文案查看不同字体。完整操作说明见 [使用指南](docs/USER.md)。

字体文件仅读取，移除来源不会删除文件。设置保存在 `%LocalAppData%\FontLens\settings.json`，SQLite 索引与覆盖缓存保存在同目录的 `index.db`。浏览、预览和统计无需网络，程序不请求网络。

界面默认使用 CPU 绘制；需要 GPU 界面加速时，可在启动前设置环境变量 `MYGO_GPU=1`。

## 构建与验证

安装 **PowerShell 7** 和 **Go 1.27.3**，在项目根目录执行：

```powershell
./scripts/build.ps1
```

首次构建需要下载 Go 依赖。脚本设置 `CGO_ENABLED=0` 和 `GOFLAGS=-mod=readonly`，清理目标输出目录，再使用锁定的 MyGo v0.8.3 构建。产物为 `dist/windows-amd64/FontLens.exe`，图标与应用数据内嵌，分发时只需该 EXE。

提交前执行静态检查与内置自检：

```powershell
go vet ./...
go tool mygo vet
./dist/windows-amd64/FontLens.exe --self-check
./dist/windows-amd64/FontLens.exe --version
```

自检覆盖字表分母、导入校验、名称排序与去重、系统字体枚举、字体读取、预览及缓存。也可使用 `FontLens.exe --self-check "C:\Fonts\example.ttf" "FontLens 字镜 123"` 检查指定字体。自检失败时进程返回非零退出码。

## 开发与发布

变更通过面向 `main` 的 PR 合并，PR 标题遵循 Angular Commit Message Guidelines。Windows CI 执行格式检查、Go/MyGo 静态检查、便携单 EXE 构建、版本校验、自检与启动检查，并保留 7 天构建产物。

`mygo.json` 是版本号的唯一来源，当前版本为 `0.1.0`。正式发布时，通过版本 PR 更新版本号并添加 `releases/vX.Y.Z.md`；合并后 CI 构建并验证该提交，创建 Release 并上传 `FontLens-vX.Y.Z-win-x64.exe`。普通 PR 不触发正式发布。

## 源码结构

| 路径 | 职责 |
| --- | --- |
| `main.go`、`internal/app/` | 入口、应用生命周期、原生界面、设置与自检 |
| `internal/fonts/` | 字体枚举、解析、属性与原始表读取 |
| `internal/preview/` | DirectWrite 预览渲染与文学文案 |
| `internal/coverage/`、`internal/codepoints/` | 内置与导入字表、覆盖统计、码位集合 |
| `internal/store/` | SQLite 字体索引与覆盖缓存 |
| `scripts/` | 便携构建与字表再生成 |

## 数据与许可

项目源码采用 [MIT 许可证](LICENSE)。内置字表保存在 `internal/coverage/data/`，版本、计数口径、来源和再生成方法见 [内置字表](docs/CHARSETS.md)。GB18030 级别 3 包含 2024 年修改单；Unicode 区块使用 17.0 数据，分母排除未分配、代理和私用码位，私用区单列。

文学文案的原作和出处见 [预览文案出处](docs/PREVIEW_TEXTS.md)，依赖和数据许可见 [第三方许可](third_party/licenses/README.md)。字体文件由各自权利人授权，本仓库不分发测试字体。
