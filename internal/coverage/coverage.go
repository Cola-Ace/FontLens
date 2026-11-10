package coverage

import (
	_ "embed"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"fontlens/internal/codepoints"
)

//go:embed data/gb2312.txt
var gb2312 string

//go:embed data/gbk.txt
var gbk string

//go:embed data/gb18030-2022-1.txt
var gb18030Level1 string

//go:embed data/gb18030-2022-2.txt
var gb18030Level2 string

//go:embed data/gb18030-2022-3.txt
var gb18030Level3 string

//go:embed data/tgh-2013.txt
var tgh string

//go:embed data/tgh-2013-1.txt
var tghLevel1 string

//go:embed data/tgh-2013-2.txt
var tghLevel2 string

//go:embed data/tgh-2013-3.txt
var tghLevel3 string

//go:embed data/big5.txt
var big5 string

//go:embed data/hkscs-2016.txt
var hkscs string

//go:embed data/jis0208-1990.txt
var jis0208 string

//go:embed data/jis0213-2004.txt
var jis0213 string

//go:embed data/ks1001-1992.txt
var ks1001 string

//go:embed data/hangul-syllables.txt
var hangul string

//go:embed data/blocks-17.tsv
var blockData string

type Row struct {
	Name    string
	Covered int
	Total   int
	Missing int
	Percent float64
	Pending bool
}

type Result struct {
	Charsets   []Row
	Blocks     []Row
	Codepoints int
}

type block struct {
	name  string
	start rune
	end   rune
	total int
}

var (
	blocksOnce        sync.Once
	blocks            []block
	privateTotalValue int
)

func Analyze(characters codepoints.Set) Result {
	return AnalyzeWith(characters, nil)
}

func AnalyzeWith(characters codepoints.Set, imported []Definition) Result {
	blocksOnce.Do(loadBlocks)
	result := Result{Charsets: Builtin(characters), Codepoints: characters.Count()}
	for _, definition := range imported {
		result.Charsets = append(result.Charsets, measure(definition.Name, definition.Members, characters))
	}
	counts := make([]int, len(blocks))
	privateCount := 0
	for r := range characters.All() {
		if unicode.Is(unicode.Co, r) {
			privateCount++
			continue
		}
		if unicode.Is(unicode.Cn, r) || unicode.Is(unicode.Cs, r) {
			continue
		}
		index := sort.Search(len(blocks), func(i int) bool { return blocks[i].end >= r })
		if index < len(blocks) && blocks[index].start <= r {
			counts[index]++
		}
	}
	for i, b := range blocks {
		if b.total == 0 {
			continue
		}
		result.Blocks = append(result.Blocks, makeRow(b.name, counts[i], b.total))
	}
	result.Blocks = append(result.Blocks, makeRow("私用区", privateCount, privateTotalValue))
	sort.SliceStable(result.Blocks, func(i, j int) bool {
		return blockPriority(result.Blocks[i].Name) < blockPriority(result.Blocks[j].Name)
	})
	return result
}

func blockPriority(name string) int {
	if strings.HasPrefix(name, "CJK") || name == "Kangxi Radicals" {
		return 0
	}
	if name == "私用区" {
		return 1
	}
	return 2
}

func loadBlocks() {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if unicode.Is(unicode.Co, r) {
			privateTotalValue++
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(blockData), "\n") {
		span, name, ok := strings.Cut(line, ";")
		if !ok {
			continue
		}
		startText, endText, ok := strings.Cut(span, "..")
		if !ok {
			continue
		}
		start, _ := strconv.ParseUint(startText, 16, 32)
		end, _ := strconv.ParseUint(endText, 16, 32)
		b := block{name: strings.TrimSpace(name), start: rune(start), end: rune(end)}
		for r := b.start; r <= b.end; r++ {
			if !unicode.Is(unicode.Cn, r) && !unicode.Is(unicode.Cs, r) && !unicode.Is(unicode.Co, r) {
				b.total++
			}
		}
		blocks = append(blocks, b)
	}
}

func Builtin(characters codepoints.Set) []Row {
	return []Row{
		measure("GB2312 汉字集", gb2312, characters),
		measure("GBK 汉字集", gbk, characters),
		measure("GB18030-2022 级别 1（汉字及部首）", gb18030Level1, characters),
		measure("GB18030-2022 级别 2（汉字及部首）", gb18030Level2, characters),
		measure("GB18030-2022 级别 3（2024 修订）", gb18030Level3, characters),
		measure("通用规范汉字表（2013，全部）", tgh, characters),
		measure("通用规范汉字表（2013，一级）", tghLevel1, characters),
		measure("通用规范汉字表（2013，二级）", tghLevel2, characters),
		measure("通用规范汉字表（2013，三级）", tghLevel3, characters),
		measure("Big5 字符集（Python 3.12 映射）", big5, characters),
		measure("HKSCS-2016 增补字符集", hkscs, characters),
		measure("日文 JIS X 0208:1990", jis0208, characters),
		measure("日文 JIS X 0213:2004", jis0213, characters),
		measure("韩文 KS X 1001:1992", ks1001, characters),
		measure("韩文音节（Unicode 17.0）", hangul, characters),
	}
}

func measure(name, members string, characters codepoints.Set) Row {
	row := Row{Name: name}
	for _, r := range members {
		row.Total++
		if characters.Contains(r) {
			row.Covered++
		}
	}
	return makeRow(row.Name, row.Covered, row.Total)
}

func makeRow(name string, covered, total int) Row {
	return Row{Name: name, Covered: covered, Total: total, Missing: total - covered, Percent: float64(covered) * 100 / float64(total)}
}
