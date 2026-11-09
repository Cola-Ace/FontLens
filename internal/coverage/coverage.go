package coverage

import (
	_ "embed"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

//go:embed data/gb2312.txt
var gb2312 string

//go:embed data/gbk.txt
var gbk string

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

func Analyze(characters map[rune]struct{}) Result {
	return AnalyzeWith(characters, nil)
}

func AnalyzeWith(characters map[rune]struct{}, imported []Definition) Result {
	blocksOnce.Do(loadBlocks)
	result := Result{Charsets: Builtin(characters), Codepoints: len(characters)}
	for _, definition := range imported {
		result.Charsets = append(result.Charsets, measure(definition.Name, definition.Members, characters))
	}
	counts := make([]int, len(blocks))
	privateCount := 0
	for r := range characters {
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

func Builtin(characters map[rune]struct{}) []Row {
	return []Row{
		measure("GB2312 汉字集", gb2312, characters),
		measure("GBK 汉字集", gbk, characters),
		{Name: "GB18030 分级字表（版本与等级待导入）", Pending: true},
		{Name: "通用规范汉字表（版本待导入）", Pending: true},
		{Name: "Big5 / HKSCS 字表（版本待导入）", Pending: true},
		{Name: "日韩字表（版本待导入）", Pending: true},
	}
}

func measure(name, members string, characters map[rune]struct{}) Row {
	row := Row{Name: name}
	for _, r := range members {
		row.Total++
		if _, ok := characters[r]; ok {
			row.Covered++
		}
	}
	return makeRow(row.Name, row.Covered, row.Total)
}

func makeRow(name string, covered, total int) Row {
	return Row{Name: name, Covered: covered, Total: total, Missing: total - covered, Percent: float64(covered) * 100 / float64(total)}
}
