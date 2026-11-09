package fonts

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/go-text/typesetting/font/opentype"
)

type Property struct {
	Group string
	Name  string
	Value string
}

func readProperties(loader *opentype.Loader) []Property {
	var result []Property
	add := func(group, name string, value any) {
		result = append(result, Property{Group: group, Name: name, Value: fmt.Sprint(value)})
	}
	if data, err := loader.RawTable(opentype.MustNewTag("head")); err == nil {
		if len(data) < 54 {
			add("head", "状态", "解析失败：表长度不足")
		} else {
			add("head", "版本", fmt.Sprintf("%d.%d", binary.BigEndian.Uint16(data[0:2]), binary.BigEndian.Uint16(data[2:4])))
			add("head", "fontRevision", float32(int32(binary.BigEndian.Uint32(data[4:8])))/65536)
			add("head", "checksumAdjustment", fmt.Sprintf("0x%08X", binary.BigEndian.Uint32(data[8:12])))
			add("head", "flags", fmt.Sprintf("0x%04X", binary.BigEndian.Uint16(data[16:18])))
			add("head", "每 em 设计单位", binary.BigEndian.Uint16(data[18:20]))
			add("head", "创建时间（字体声明）", macTime(data[20:28]))
			add("head", "修改时间（字体声明）", macTime(data[28:36]))
			add("head", "边界框", fmt.Sprintf("%d, %d, %d, %d", int16(binary.BigEndian.Uint16(data[36:38])), int16(binary.BigEndian.Uint16(data[38:40])), int16(binary.BigEndian.Uint16(data[40:42])), int16(binary.BigEndian.Uint16(data[42:44]))))
			add("head", "macStyle", fmt.Sprintf("0x%04X", binary.BigEndian.Uint16(data[44:46])))
			add("head", "loca 格式", int16(binary.BigEndian.Uint16(data[50:52])))
		}
	}
	if data, err := loader.RawTable(opentype.MustNewTag("OS/2")); err == nil {
		if len(data) < 78 {
			add("OS/2", "状态", "解析失败：表长度不足")
		} else {
			add("OS/2", "版本", binary.BigEndian.Uint16(data[0:2]))
			add("OS/2", "平均字宽", int16(binary.BigEndian.Uint16(data[2:4])))
			add("OS/2", "字重等级", binary.BigEndian.Uint16(data[4:6]))
			add("OS/2", "字宽等级", binary.BigEndian.Uint16(data[6:8]))
			rights := binary.BigEndian.Uint16(data[8:10])
			add("OS/2", "嵌入权限 fsType", fmt.Sprintf("0x%04X · %s", rights, embeddingRights(rights)))
			add("OS/2", "下标 xSize / ySize / xOffset / yOffset", fmt.Sprintf("%d / %d / %d / %d", int16(binary.BigEndian.Uint16(data[10:12])), int16(binary.BigEndian.Uint16(data[12:14])), int16(binary.BigEndian.Uint16(data[14:16])), int16(binary.BigEndian.Uint16(data[16:18]))))
			add("OS/2", "上标 xSize / ySize / xOffset / yOffset", fmt.Sprintf("%d / %d / %d / %d", int16(binary.BigEndian.Uint16(data[18:20])), int16(binary.BigEndian.Uint16(data[20:22])), int16(binary.BigEndian.Uint16(data[22:24])), int16(binary.BigEndian.Uint16(data[24:26]))))
			add("OS/2", "删除线粗细 / 位置", fmt.Sprintf("%d / %d", int16(binary.BigEndian.Uint16(data[26:28])), int16(binary.BigEndian.Uint16(data[28:30]))))
			add("OS/2", "IBM family class", fmt.Sprintf("0x%04X", binary.BigEndian.Uint16(data[30:32])))
			add("OS/2", "PANOSE", fmt.Sprintf("% X", data[32:42]))
			add("OS/2", "声明 Unicode 范围", fmt.Sprintf("% X", data[42:58]))
			add("OS/2", "厂商 ID", string(data[58:62]))
			add("OS/2", "fsSelection", fmt.Sprintf("0x%04X", binary.BigEndian.Uint16(data[62:64])))
			add("OS/2", "首字符索引", binary.BigEndian.Uint16(data[64:66]))
			add("OS/2", "末字符索引", binary.BigEndian.Uint16(data[66:68]))
			add("OS/2", "Typo ascender / descender / line gap", fmt.Sprintf("%d / %d / %d", int16(binary.BigEndian.Uint16(data[68:70])), int16(binary.BigEndian.Uint16(data[70:72])), int16(binary.BigEndian.Uint16(data[72:74]))))
			add("OS/2", "Win ascent / descent", fmt.Sprintf("%d / %d", binary.BigEndian.Uint16(data[74:76]), binary.BigEndian.Uint16(data[76:78])))
			if len(data) >= 86 {
				add("OS/2", "声明代码页范围", fmt.Sprintf("% X", data[78:86]))
			}
			if len(data) >= 90 {
				add("OS/2", "x-height / cap height", fmt.Sprintf("%d / %d", int16(binary.BigEndian.Uint16(data[86:88])), int16(binary.BigEndian.Uint16(data[88:90]))))
			}
		}
	}
	for _, tag := range []string{"hhea", "vhea"} {
		if data, err := loader.RawTable(opentype.MustNewTag(tag)); err == nil {
			if len(data) < 36 {
				add(tag, "状态", "解析失败：表长度不足")
			} else {
				add(tag, "ascender / descender / line gap", fmt.Sprintf("%d / %d / %d", int16(binary.BigEndian.Uint16(data[4:6])), int16(binary.BigEndian.Uint16(data[6:8])), int16(binary.BigEndian.Uint16(data[8:10]))))
				add(tag, "最大 advance", binary.BigEndian.Uint16(data[10:12]))
				add(tag, "最小前侧 / 后侧 bearing", fmt.Sprintf("%d / %d", int16(binary.BigEndian.Uint16(data[12:14])), int16(binary.BigEndian.Uint16(data[14:16]))))
				add(tag, "最大 extent", int16(binary.BigEndian.Uint16(data[16:18])))
				add(tag, "caret slope rise / run", fmt.Sprintf("%d / %d", int16(binary.BigEndian.Uint16(data[18:20])), int16(binary.BigEndian.Uint16(data[20:22]))))
				add(tag, "长度量记录数", binary.BigEndian.Uint16(data[34:36]))
			}
		}
	}
	if data, err := loader.RawTable(opentype.MustNewTag("post")); err == nil {
		if len(data) < 16 {
			add("post", "状态", "解析失败：表长度不足")
		} else {
			add("post", "斜体角度", float32(int32(binary.BigEndian.Uint32(data[4:8])))/65536)
			add("post", "版本", fmt.Sprintf("0x%08X", binary.BigEndian.Uint32(data[0:4])))
			add("post", "下划线位置 / 厚度", fmt.Sprintf("%d / %d", int16(binary.BigEndian.Uint16(data[8:10])), int16(binary.BigEndian.Uint16(data[10:12]))))
			add("post", "等宽标志", binary.BigEndian.Uint32(data[12:16]) != 0)
		}
	}
	return result
}

func macTime(data []byte) string {
	seconds := binary.BigEndian.Uint64(data)
	if seconds == 0 || seconds > 1<<63-1 {
		return "未提供"
	}
	return time.Unix(int64(seconds)-2082844800, 0).UTC().Format("2006-01-02 15:04:05 UTC")
}

func embeddingRights(value uint16) string {
	if value == 0 {
		return "可安装嵌入"
	}
	var labels []string
	for _, rule := range []struct {
		bit  uint16
		name string
	}{{2, "受限许可"}, {4, "仅预览与打印"}, {8, "可编辑"}, {0x0100, "禁止子集"}, {0x0200, "仅位图"}} {
		if value&rule.bit != 0 {
			labels = append(labels, rule.name)
		}
	}
	if len(labels) == 0 {
		return "未定义标志"
	}
	return strings.Join(labels, "、")
}
