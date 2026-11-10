# 内置字表

字符集覆盖按 Unicode 码位去重，检查字体 Unicode `cmap` 是否映射到非零 glyph ID。下表数据随程序内置，浏览字体时无需下载。

| 字表 | 版本及统计范围 | 码位数 |
| --- | --- | ---: |
| GB2312 汉字集 | Python 3.12 `gb2312` 映射中的汉字 | 6,763 |
| GBK 汉字集 | Python 3.12 `gbk` 映射中的汉字 | 20,923 |
| GB18030 级别 1 | GB 18030-2022，汉字及部首 | 27,584 |
| GB18030 级别 2 | 级别 1 加通规表中额外的 196 字 | 27,780 |
| GB18030 级别 3 | GB 18030-2022 含 2024 年第 1 号修改单，97,680 个汉字及 228 个部首 | 97,908 |
| 通用规范汉字表 | 2013，全部 | 8,105 |
| 通用规范汉字表一级 | 2013，序号 1–3500 | 3,500 |
| 通用规范汉字表二级 | 2013，序号 3501–6500 | 3,000 |
| 通用规范汉字表三级 | 2013，序号 6501–8105 | 1,605 |
| Big5 | Python 3.12 `big5` 双字节映射，含非汉字 | 13,706 |
| HKSCS | HKSCS-2016 增补字符，含非汉字及 2018 年映射修订 | 5,033 |
| 日文 JIS X 0208 | 1990，Unicode 映射表 v2.0 | 6,879 |
| 日文 JIS X 0213 | 2004，Project X0213 标准映射，2009-05-03 | 11,209 |
| 韩文 KS X 1001 | 1992（原 KS C 5601），Unicode 映射表 v1.1 | 8,224 |
| 韩文音节 | Unicode 17.0，U+AC00–U+D7A3 | 11,172 |

GB18030 三个级别逐级包含；其分母只含相应级别的汉字和部首。级别 1 使用原版标准的 U+3400–U+4DB5、U+4E00–U+9FEF、12 个兼容汉字和双字节部分的 14 个部首。2024 年修改单新增的 9,793 个汉字全部计入级别 3。原版级别 3 的 88,115 个汉字及部首加上这些新增字符，即当前的 97,908 个码位。实现级别及修改单依据见 [Unicode 对 2022 版的说明](https://www.unicode.org/L2/L2022/22274-disruptive-changes.pdf)、[修改单编码范围](https://www.unicode.org/L2/L2023/23271-gb18030-2022-amd-draft3.pdf)和[赛西认证修改单执行通知](https://www.cc.cesi.cn/news/show-4889.aspx)。

通规表的三个级别互不重叠，“全部”为三者并集。字符及序号提取自固定版本 [Unicode 17.0 Unihan](https://www.unicode.org/Public/17.0.0/ucd/Unihan.zip) 的 `kTGH` 属性，对应 [2013 年《通用规范汉字表》](https://www.gov.cn/gzdt/att/att/site1/20130819/tygfhzb.pdf)。

Big5 与 HKSCS 分别统计基础字符集和香港增补字符集，二者有重叠。Big5 映射的 13,710 个编码位置去重后为 13,706 个码位；使用 [CPython 3.12 的映射](https://github.com/python/cpython/blob/v3.12.5/Modules/cjkcodecs/mappings_tw.h)，不包含 CP950 或 WHATWG 的其他扩展。HKSCS 来自[香港特区政府数字政策办公室通过 DATA.GOV.HK 提供的开放数据](https://data.gov.hk/en-data/dataset/hk-dpo-dpo_hp-hong-kong-supplementary-character-set-related-information)，使用其 [JSON](https://www.digitalpolicy.gov.hk/open_data/ccli/HKSCS2016.json) 中的全部 5,033 个 `codepoint`。该数据已将 H-8FA8 从 U+2F9B2 修订至 U+270F0，未加入 2019 年以后的新增字符。

日文与韩文字表保留假名、韩文音节、汉字和符号。JIS X 0208 使用 [Unicode JIS0208.TXT](https://www.unicode.org/Public/MAPPINGS/OBSOLETE/EASTASIA/JIS/JIS0208.TXT)；JIS X 0213 使用 [Project X0213 标准映射](https://x0213.org/codetable/jisx0213-2004-std.txt)。后者共有 11,233 个编码位置，其中 25 个映射为双码位序列，拆分并去重后得到 11,209 个码位。这里检查各码位的 `cmap` 覆盖；组合序列的排版效果需在字体预览中查看。

KS X 1001 从 [Unicode KSC5601.TXT](https://www.unicode.org/Public/MAPPINGS/OBSOLETE/EASTASIA/KSC/KSC5601.TXT) 中只取两个字节均在 A1–FE 的标准区域，保留 U+3164 韩文填充字符，排除 UHC 扩展区；其中含 2,350 个韩文音节。完整现代韩文音节另列 11,172 个码位。

## 重新生成

在项目根目录使用 Python 3.12：

```powershell
python scripts/generate-charsets.py
python scripts/generate-charsets.py --check
```

脚本下载固定版本源文件至 `.cache/charsets/`，生成 `internal/coverage/data/` 下的 UTF-8 字表，并校验总数、通规序号、GB18030 级别包含关系和补充平面字符。`--check` 只比较数据。字表按码位排序，无 BOM、分隔符或结尾换行；空格、全角空格等正式成员按原码位保留。

GB18030 的 14 个部首映射提取自 [ICU release-76-1 的 GB18030-2022 映射](https://github.com/unicode-org/icu/blob/release-76-1/icu4c/source/data/mappings/gb18030-2022.ucm)，其余分级范围按标准固定。已有 GB2312、GBK 数据保留原生成结果。数据许可及署名见 [第三方许可](../third_party/licenses/README.md)。
