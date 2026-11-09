package fonts

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	webfont "github.com/tdewolff/font"
)

type NameRecord struct {
	ID       uint16
	Platform uint16
	Encoding uint16
	Language uint16
	Value    string
}

type Table struct {
	Tag      string
	Size     int
	Offset   uint32
	Checksum uint32
}

type Axis struct {
	Tag     string
	Name    string
	Minimum float32
	Default float32
	Maximum float32
}

type NamedInstance struct {
	Name        string
	Coordinates map[string]float32
}

type Face struct {
	Path       string
	Size       int64
	Modified   int64
	Index      int
	Count      int
	Format     string
	FullName   string
	Family     string
	Subfamily  string
	PostScript string
	Names      []NameRecord
	Tables     []Table
	Glyphs     uint16
	Cmap       string
	Characters map[rune]struct{}
	Axes       []Axis
	Instances  []NamedInstance
	Properties []Property
}

func (f Face) ID() string { return fmt.Sprintf("%s#%d", strings.ToLower(f.Path), f.Index) }

func DecodeWebFont(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(path), ".woff2") {
		return webfont.ParseWOFF2(data)
	}
	return webfont.ParseWOFF(data)
}

func Read(path string) ([]Face, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	var loaders []*opentype.Loader
	var directorySource io.ReaderAt = file
	if strings.EqualFold(filepath.Ext(path), ".woff2") || strings.EqualFold(filepath.Ext(path), ".woff") {
		sfnt, decodeErr := DecodeWebFont(path)
		if decodeErr != nil {
			return nil, decodeErr
		}
		reader := bytes.NewReader(sfnt)
		directorySource = reader
		loaders, err = opentype.NewLoaders(reader)
	} else {
		loaders, err = opentype.NewLoaders(file)
	}
	if err != nil {
		return nil, err
	}
	result := make([]Face, 0, len(loaders))
	for index, loader := range loaders {
		face := Face{Path: path, Size: info.Size(), Modified: info.ModTime().UnixNano(), Index: index, Count: len(loaders), Format: strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))}
		directory, directoryErr := readTableDirectory(directorySource, index)
		if directoryErr != nil {
			return nil, fmt.Errorf("face %d directory: %w", index, directoryErr)
		}
		for _, tag := range loader.Tables() {
			data, readErr := loader.RawTable(tag)
			if readErr != nil {
				return nil, fmt.Errorf("face %d table %s: %w", index, tag, readErr)
			}
			record, exists := directory[tag.String()]
			if !exists {
				return nil, fmt.Errorf("face %d table %s missing from directory", index, tag)
			}
			if record.Size != len(data) {
				return nil, fmt.Errorf("face %d table %s length mismatch", index, tag)
			}
			face.Tables = append(face.Tables, record)
		}
		face.Properties = readProperties(loader)
		if data, readErr := loader.RawTable(opentype.MustNewTag("name")); readErr == nil {
			face.Names = readNames(data)
			face.FullName = preferredName(face.Names, 4)
			face.Family = preferredName(face.Names, 1)
			face.Subfamily = preferredName(face.Names, 2)
			face.PostScript = preferredName(face.Names, 6)
		}
		if face.FullName == "" {
			face.FullName = fmt.Sprintf("名称未提供 · %s · 面 %d", filepath.Base(path), index+1)
		}
		if data, readErr := loader.RawTable(opentype.MustNewTag("maxp")); readErr == nil {
			if maxp, _, parseErr := tables.ParseMaxp(data); parseErr == nil {
				face.Glyphs = maxp.NumGlyphs
			}
		}
		if data, readErr := loader.RawTable(opentype.MustNewTag("cmap")); readErr == nil {
			face.Characters, face.Cmap, err = readCmap(data)
			if err != nil {
				return nil, fmt.Errorf("face %d cmap: %w", index, err)
			}
		}
		if data, readErr := loader.RawTable(opentype.MustNewTag("fvar")); readErr == nil {
			face.Axes, face.Instances = readVariations(data, face.Names)
		}
		result = append(result, face)
	}
	return result, nil
}

func readTableDirectory(source io.ReaderAt, faceIndex int) (map[string]Table, error) {
	header := make([]byte, 12)
	if _, err := source.ReadAt(header, 0); err != nil {
		return nil, err
	}
	offset := int64(0)
	if string(header[:4]) == "ttcf" {
		count := binary.BigEndian.Uint32(header[8:12])
		if faceIndex < 0 || uint32(faceIndex) >= count {
			return nil, fmt.Errorf("collection face %d is out of range", faceIndex)
		}
		entry := make([]byte, 4)
		if _, err := source.ReadAt(entry, 12+int64(faceIndex)*4); err != nil {
			return nil, err
		}
		offset = int64(binary.BigEndian.Uint32(entry))
		if _, err := source.ReadAt(header, offset); err != nil {
			return nil, err
		}
	}
	count := int(binary.BigEndian.Uint16(header[4:6]))
	if count > 4096 {
		return nil, fmt.Errorf("too many font tables: %d", count)
	}
	records := make([]byte, count*16)
	if _, err := source.ReadAt(records, offset+12); err != nil {
		return nil, err
	}
	result := make(map[string]Table, count)
	for i := 0; i < count; i++ {
		record := records[i*16:]
		tag := string(record[:4])
		result[tag] = Table{
			Tag: tag, Checksum: binary.BigEndian.Uint32(record[4:8]),
			Offset: binary.BigEndian.Uint32(record[8:12]), Size: int(binary.BigEndian.Uint32(record[12:16])),
		}
	}
	return result, nil
}

func readVariations(data []byte, names []NameRecord) ([]Axis, []NamedInstance) {
	if len(data) < 16 {
		return nil, nil
	}
	start := int(binary.BigEndian.Uint16(data[4:6]))
	axisCount := int(binary.BigEndian.Uint16(data[8:10]))
	axisSize := int(binary.BigEndian.Uint16(data[10:12]))
	instanceCount := int(binary.BigEndian.Uint16(data[12:14]))
	instanceSize := int(binary.BigEndian.Uint16(data[14:16]))
	if axisSize < 20 || axisCount > 64 || instanceCount > 1024 || start > len(data) || axisCount*axisSize > len(data)-start || instanceSize < axisCount*4+4 {
		return nil, nil
	}
	axes := make([]Axis, 0, axisCount)
	fixed := func(raw []byte) float32 { return float32(int32(binary.BigEndian.Uint32(raw))) / 65536 }
	for i := 0; i < axisCount; i++ {
		record := data[start+i*axisSize:]
		tag := string(record[:4])
		axes = append(axes, Axis{
			Tag: tag, Name: preferredName(names, binary.BigEndian.Uint16(record[18:20])),
			Minimum: fixed(record[4:8]), Default: fixed(record[8:12]), Maximum: fixed(record[12:16]),
		})
	}
	instances := make([]NamedInstance, 0, instanceCount)
	start += axisCount * axisSize
	for i := 0; i < instanceCount && start+instanceSize <= len(data); i++ {
		record := data[start : start+instanceSize]
		instance := NamedInstance{Name: preferredName(names, binary.BigEndian.Uint16(record[:2])), Coordinates: make(map[string]float32)}
		for axis := range axes {
			instance.Coordinates[axes[axis].Tag] = fixed(record[4+axis*4:])
		}
		instances = append(instances, instance)
		start += instanceSize
	}
	return axes, instances
}

func ReadRawTable(path string, faceIndex int, tag string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var resource opentype.Resource = file
	if strings.EqualFold(filepath.Ext(path), ".woff2") || strings.EqualFold(filepath.Ext(path), ".woff") {
		data, decodeErr := DecodeWebFont(path)
		if decodeErr != nil {
			return nil, decodeErr
		}
		resource = bytes.NewReader(data)
	}
	loaders, err := opentype.NewLoaders(resource)
	if err != nil {
		return nil, err
	}
	if faceIndex < 0 || faceIndex >= len(loaders) {
		return nil, fmt.Errorf("face index %d is out of range", faceIndex)
	}
	return loaders[faceIndex].RawTable(opentype.MustNewTag(tag))
}

func readNames(data []byte) []NameRecord {
	if len(data) < 6 {
		return nil
	}
	count := int(binary.BigEndian.Uint16(data[2:4]))
	base := int(binary.BigEndian.Uint16(data[4:6]))
	result := make([]NameRecord, 0, count)
	for i := 0; i < count && 6+(i+1)*12 <= len(data); i++ {
		record := data[6+i*12:]
		start := base + int(binary.BigEndian.Uint16(record[10:12]))
		length := int(binary.BigEndian.Uint16(record[8:10]))
		if start < 0 || length < 0 || start > len(data) || length > len(data)-start {
			continue
		}
		name := NameRecord{
			Platform: binary.BigEndian.Uint16(record[0:2]),
			Encoding: binary.BigEndian.Uint16(record[2:4]),
			Language: binary.BigEndian.Uint16(record[4:6]),
			ID:       binary.BigEndian.Uint16(record[6:8]),
		}
		raw := data[start : start+length]
		if name.Platform == 0 || name.Platform == 3 {
			if len(raw)%2 != 0 {
				continue
			}
			units := make([]uint16, len(raw)/2)
			for j := range units {
				units[j] = binary.BigEndian.Uint16(raw[j*2:])
			}
			name.Value = string(utf16.Decode(units))
		} else if name.Platform == 1 && name.Encoding == 0 {
			name.Value = tables.DecodeMacintosh(raw)
		} else if utf8.Valid(raw) {
			name.Value = string(raw)
		}
		if name.Value != "" {
			result = append(result, name)
		}
	}
	return result
}

func preferredName(records []NameRecord, id uint16) string {
	best, score := "", -1
	for _, record := range records {
		if record.ID != id {
			continue
		}
		n := 0
		switch {
		case record.Platform == 3 && record.Language == 0x0804:
			n = 4
		case record.Platform == 3 && record.Language == 0x0409:
			n = 3
		case record.Platform == 1 && record.Language == 0:
			n = 2
		default:
			n = 1
		}
		if n > score {
			best, score = record.Value, n
		}
	}
	return best
}

var cmapPreference = [][2]uint16{{3, 10}, {0, 6}, {0, 4}, {3, 1}, {0, 3}, {0, 2}, {0, 1}, {0, 0}}

func LookupGlyph(path string, faceIndex int, character rune) (uint16, error) {
	data, err := ReadRawTable(path, faceIndex, "cmap")
	if err != nil {
		return 0, err
	}
	table, _, err := tables.ParseCmap(data)
	if err != nil {
		return 0, err
	}
	for _, wanted := range cmapPreference {
		for _, record := range table.Records {
			if uint16(record.PlatformID) != wanted[0] || uint16(record.EncodingID) != wanted[1] {
				continue
			}
			mapping, _, processErr := font.ProcessCmap(tables.Cmap{Records: []tables.EncodingRecord{record}}, tables.FPNone)
			if processErr != nil {
				return 0, processErr
			}
			if mapping == nil {
				continue
			}
			glyph, ok := mapping.Lookup(character)
			if !ok || glyph == 0 {
				return 0, nil
			}
			return uint16(glyph), nil
		}
	}
	return 0, nil
}

func GlyphMetrics(path string, faceIndex int, glyph uint16) (uint16, int16, error) {
	hhea, err := ReadRawTable(path, faceIndex, "hhea")
	if err != nil {
		return 0, 0, err
	}
	hmtx, err := ReadRawTable(path, faceIndex, "hmtx")
	if err != nil {
		return 0, 0, err
	}
	if len(hhea) < 36 {
		return 0, 0, fmt.Errorf("hhea 表长度不足")
	}
	count := int(binary.BigEndian.Uint16(hhea[34:36]))
	if count == 0 || len(hmtx) < count*4 {
		return 0, 0, fmt.Errorf("hmtx 表长度不足")
	}
	advance := binary.BigEndian.Uint16(hmtx[min(int(glyph), count-1)*4:])
	bearingOffset := int(glyph)*4 + 2
	if int(glyph) >= count {
		bearingOffset = count*4 + (int(glyph)-count)*2
	}
	if len(hmtx) < bearingOffset+2 {
		return 0, 0, fmt.Errorf("glyph %d 超出 hmtx 表范围", glyph)
	}
	return advance, int16(binary.BigEndian.Uint16(hmtx[bearingOffset:])), nil
}

func readCmap(data []byte) (map[rune]struct{}, string, error) {
	table, _, err := tables.ParseCmap(data)
	if err != nil {
		return nil, "", err
	}
	for _, wanted := range cmapPreference {
		for _, record := range table.Records {
			if uint16(record.PlatformID) != wanted[0] || uint16(record.EncodingID) != wanted[1] {
				continue
			}
			one := tables.Cmap{Records: []tables.EncodingRecord{record}}
			mapping, _, processErr := font.ProcessCmap(one, tables.FPNone)
			if processErr != nil {
				return nil, "", processErr
			}
			if mapping == nil {
				continue
			}
			chars := make(map[rune]struct{})
			iter := mapping.Iter()
			for iter.Next() {
				r, glyph := iter.Char()
				if glyph != 0 && utf8.ValidRune(r) && !(r >= 0xD800 && r <= 0xDFFF) {
					chars[r] = struct{}{}
				}
			}
			return chars, fmt.Sprintf("(%d, %d)", wanted[0], wanted[1]), nil
		}
	}
	return nil, "未提供", nil
}

func Scan(ctx context.Context, root string, read func(string) ([]Face, error), directory func(string), visit func(Face), report func(string, error)) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			report(path, err)
			return nil
		}
		if entry.IsDir() {
			directory(path)
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ttf" && ext != ".otf" && ext != ".ttc" && ext != ".otc" && ext != ".woff" && ext != ".woff2" {
			return nil
		}
		faces, readErr := read(path)
		if readErr != nil {
			report(path, readErr)
			return nil
		}
		for _, face := range faces {
			visit(face)
		}
		return nil
	})
}
