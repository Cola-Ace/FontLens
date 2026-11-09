package codepoints

import (
	"iter"
	"slices"
	"sort"
)

type Range struct {
	First rune
	Last  rune
}

// Set stores sorted, disjoint ranges of Unicode codepoints.
type Set []Range

// New sorts characters in place and merges adjacent and duplicate codepoints.
func New(characters []rune) Set {
	slices.Sort(characters)
	result := make(Set, 0)
	for _, r := range characters {
		if len(result) > 0 {
			last := &result[len(result)-1]
			if r <= last.Last+1 {
				last.Last = max(last.Last, r)
				continue
			}
		}
		result = append(result, Range{First: r, Last: r})
	}
	return result
}

func (s Set) Count() int {
	count := 0
	for _, span := range s {
		count += int(span.Last-span.First) + 1
	}
	return count
}

func (s Set) Contains(r rune) bool {
	i := sort.Search(len(s), func(i int) bool { return s[i].Last >= r })
	return i < len(s) && s[i].First <= r
}

func (s Set) All() iter.Seq[rune] {
	return func(yield func(rune) bool) {
		for _, span := range s {
			for r := span.First; r <= span.Last; r++ {
				if !yield(r) {
					return
				}
			}
		}
	}
}
