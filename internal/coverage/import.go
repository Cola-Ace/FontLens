package coverage

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Definition struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Members string `json:"members"`
}

func Parse(name string, data []byte) (Definition, error) {
	if !utf8.Valid(data) {
		return Definition{}, fmt.Errorf("字表必须是 UTF-8 编码")
	}
	content := strings.TrimPrefix(string(data), "\ufeff")
	definition := Definition{Name: strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)), Source: name}
	declared := -1
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		lines := strings.Split(content, "\n")
		end := -1
		inName := false
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSuffix(lines[i], "\r")
			if line == "---" {
				end = i
				break
			}
			key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
			if !ok {
				continue
			}
			value = strings.Trim(strings.TrimSpace(value), "'\"")
			if !strings.HasPrefix(line, " ") {
				inName = key == "name"
				switch key {
				case "name":
					if value != "" {
						definition.Name = value
					}
				case "version":
					definition.Version = value
				case "count":
					var err error
					declared, err = strconv.Atoi(value)
					if err != nil || declared < 0 {
						return Definition{}, fmt.Errorf("字表声明数量无效: %s", value)
					}
				}
			} else if inName && (key == "zhs" || key == "en") && value != "" {
				if key == "zhs" || definition.Name == strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)) {
					definition.Name = value
				}
			}
		}
		if end < 0 {
			return Definition{}, fmt.Errorf("YAML Front Matter 缺少结束分隔线")
		}
		content = strings.Join(lines[end+1:], "\n")
	}
	seen := make(map[rune]struct{})
	var members strings.Builder
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSuffix(line, "\r")
		for _, character := range line {
			if _, exists := seen[character]; !exists {
				seen[character] = struct{}{}
				members.WriteRune(character)
			}
		}
	}
	if len(seen) == 0 {
		return Definition{}, fmt.Errorf("字表没有字符")
	}
	if declared >= 0 && declared != len(seen) {
		return Definition{}, fmt.Errorf("字表声明 %d 字，实际去重后 %d 字", declared, len(seen))
	}
	definition.Members = members.String()
	return definition, nil
}
