# 字镜 · FontLens

字镜是 Windows 11 x64 的离线字体浏览工具。原生三栏界面通过 DirectWrite 枚举系统字体，按字体面列出系统及自定义目录中的 TTF、OTF、TTC、OTC、WOFF 和 WOFF2 文件。列表和详情绑定具体文件及集合面生成字样；可以编辑多行文案、选择文学预设、调整字号与行距、调节可变字体轴，并选择 2–4 款字体并排对比。支持全名搜索、格式筛选、自然排序、字符与字形度量查询、名称记录、原始表目录及 GB2312、GBK、Unicode 17.0 覆盖统计，也可导入 UTF-8 字表。

## 使用

启动后左侧显示系统字体。在左下角点击“添加文件夹”选择目录，或输入文件夹路径并点击该按钮，可递归读取该目录。点击目录树节点限定列表范围；搜索按字体全名匹配。点击字体面，在右侧查看名称、文件路径、集合面索引、`cmap`、覆盖数和表目录。勾选 2–4 行后点击“对比”可并排查看。移除来源只更改应用设置，不删除字体文件。

设置保存在 `%LocalAppData%\FontLens\settings.json`，SQLite 索引与覆盖缓存保存在同目录的 `index.db`。扫描仅读取字体文件，程序不请求网络。测试字体可以放在本地目录，但不随源码分发。

## 构建

使用 PowerShell 7 和 Go 1.27.3：

```powershell
$env:CGO_ENABLED = '0'
$env:GOFLAGS = '-mod=readonly'
go tool mygo build -platform windows/amd64 -skip-installer
```

产物位于 `dist/windows-amd64/FontLens.exe`。项目锁定 MyGo v0.8.3，依赖由 `go.mod` 和 `go.sum` 指定。

## 数据与许可

项目源码采用 MIT 许可证。内置的 GB2312/GBK 字符集合由 Python 3.12 标准编解码映射生成，分别保存在 `internal/coverage/data/gb2312.txt` 和 `gbk.txt`。缺少确定版本数据的其他字表标为“待导入”。Unicode 区块范围由 `@unicode/unicode-17.0.0` v2.1.0 数据生成，分母按 Go 1.27.3 的 Unicode 17.0.0 分类表排除未分配、代理和私用码位；私用区单列。文学预览文案的原作和出处见 [预览文案出处](docs/PREVIEW_TEXTS.md)。第三方许可在程序内“说明与许可证”及 `third_party/licenses/` 查看。字体文件的许可由其各自权利人规定。
