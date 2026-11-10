# 字镜 · FontLens

字镜是 Windows 11 x64 的离线字体浏览工具。原生三栏界面通过 DirectWrite 枚举系统字体，按字体面列出系统及自定义目录中的 TTF、OTF、TTC、OTC、WOFF 和 WOFF2 文件。列表使用字体自身显示固定的 Family Name，详情字样绑定具体文件及集合面；字体对比支持自定义文案、文学预设、字号与行距调整，可调节可变字体轴，并选择 2–4 款字体并排对比。支持全名搜索、格式筛选、自然排序、名称记录、原始表目录及 GB2312、GBK、GB18030、通用规范汉字表、Big5/HKSCS、日韩字表与 Unicode 17.0 覆盖统计，也可导入 UTF-8 字表。

## 使用

启动后左侧显示系统字体。在左下角点击“添加文件夹”选择目录，可递归读取该目录。点击目录树节点限定列表范围；搜索按字体全名匹配。点击字体面，在右侧查看名称、文件路径、集合面索引、`cmap`、覆盖数和表目录。左键双击字体行可打开字体文件，右键菜单中的“打开该字体所在目录”可打开对应文件夹。勾选 2–4 行后点击“对比”可并排查看。移除来源只更改应用设置，不删除字体文件。

设置保存在 `%LocalAppData%\FontLens\settings.json`，SQLite 索引与覆盖缓存保存在同目录的 `index.db`。扫描仅读取字体文件，程序不请求网络。测试字体可以放在本地目录，但不随源码分发。

字体预览由 DirectWrite 排版、Direct2D 绘制，最多两个任务并行，只保留当前页面需要的位图。界面默认使用 CPU 绘制以降低内存占用；需要 GPU 界面加速时，可在启动前设置环境变量 `MYGO_GPU=1`。

## 构建

使用 PowerShell 7 和 Go 1.27.3：

```powershell
$env:CGO_ENABLED = '0'
$env:GOFLAGS = '-mod=readonly'
go tool mygo build -platform windows/amd64 -skip-installer
```

产物位于 `dist/windows-amd64/FontLens.exe`。项目锁定 MyGo v0.8.3，依赖由 `go.mod` 和 `go.sum` 指定。

## 源码结构

根目录的 `main.go` 调用应用入口。`internal/app/` 集中管理应用生命周期、界面、设置与自检；字体读取、覆盖统计、预览渲染和缓存分别位于 `internal/fonts/`、`internal/coverage/`、`internal/preview/` 和 `internal/store/`。

## 数据与许可

项目源码采用 MIT 许可证。内置字表保存在 `internal/coverage/data/`，包含 GB18030-2022（级别 3 含 2024 年修改单）、2013 年通用规范汉字表、Big5、HKSCS-2016、JIS X 0208:1990、JIS X 0213:2004、KS X 1001:1992 与完整现代韩文音节；版本、计数口径、来源和再生成方法见 [内置字表](docs/CHARSETS.md)。Unicode 区块范围由 `@unicode/unicode-17.0.0` v2.1.0 数据生成，分母按 Go 1.27.3 的 Unicode 17.0.0 分类表排除未分配、代理和私用码位；私用区单列。文学预览文案的原作和出处见 [预览文案出处](docs/PREVIEW_TEXTS.md)。第三方许可在 `third_party/licenses/` 查看。字体文件的许可由其各自权利人规定。
