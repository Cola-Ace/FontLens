package fonts

import (
	"bytes"
	"encoding/binary"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

func legacyNameEncoding(name NameRecord) encoding.Encoding {
	if name.Platform == 3 {
		switch name.Encoding {
		case 3:
			return simplifiedchinese.GBK
		case 4:
			return traditionalchinese.Big5
		case 5:
			return korean.EUCKR
		}
	} else if name.Platform == 1 {
		switch name.Encoding {
		case 1:
			return japanese.ShiftJIS
		case 2:
			return traditionalchinese.Big5
		case 3:
			return korean.EUCKR
		case 25:
			return simplifiedchinese.GBK
		}
	}
	return nil
}

func decodeName(raw []byte, name NameRecord) (string, bool) {
	if codec := legacyNameEncoding(name); codec != nil {
		// Older font converters sometimes expanded each legacy byte to a UTF-16 unit.
		padded := len(raw) > 0 && len(raw)%2 == 0
		for i := 0; padded && i < len(raw); i += 2 {
			padded = raw[i] == 0
		}
		if padded {
			compact := make([]byte, len(raw)/2)
			for i := range compact {
				compact[i] = raw[i*2+1]
			}
			raw = compact
		}
		value, ok := decodeLegacyName(bytes.TrimRight(raw, "\x00"), codec)
		if !ok && name.Platform == 3 && name.ID == 2 {
			// Legacy CJK fonts can store the subfamily in UTF-16BE within a code-page record.
			return decodeUnicodeName(raw)
		}
		return value, ok
	}
	if name.Platform == 0 || name.Platform == 3 {
		return decodeUnicodeName(raw)
	}
	if name.Platform == 1 && name.Encoding == 0 {
		return decodeLegacyName(bytes.TrimRight(raw, "\x00"), charmap.Macintosh)
	}
	return strings.TrimRight(string(raw), "\x00"), utf8.Valid(raw)
}

func decodeUnicodeName(raw []byte) (string, bool) {
	if len(raw)%2 != 0 {
		return "", false
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.BigEndian.Uint16(raw[i*2:])
	}
	value := strings.TrimRight(string(utf16.Decode(units)), "\x00")
	return value, !strings.ContainsRune(value, utf8.RuneError) && !strings.ContainsRune(value, 0)
}

func decodeLegacyName(raw []byte, codec encoding.Encoding) (string, bool) {
	value, err := codec.NewDecoder().String(string(raw))
	return value, err == nil && !strings.ContainsRune(value, utf8.RuneError) && !strings.ContainsRune(value, 0)
}

func shadowedName(name NameRecord, records []NameRecord) bool {
	if name.Platform == 3 && (name.Encoding == 1 || name.Encoding == 10) || name.Platform == 0 {
		return shadowedUnicodeName(name, records)
	}
	if name.Platform != 1 || name.Encoding != 0 {
		return false
	}
	raw, err := charmap.Macintosh.NewEncoder().Bytes([]byte(name.Value))
	if err != nil {
		return false
	}
	for _, other := range records {
		if other.ID != name.ID || !other.Decoded || other.Value == "" || other.Value == name.Value || other.Platform != 3 {
			continue
		}
		codec := legacyNameEncoding(other)
		if codec == nil {
			switch other.Language {
			case 0x0804, 0x1004:
				codec = simplifiedchinese.GBK
			case 0x0404, 0x0C04, 0x1404:
				codec = traditionalchinese.Big5
			case 0x0411:
				codec = japanese.ShiftJIS
			case 0x0412:
				codec = korean.EUCKR
			default:
				continue
			}
		}
		// Trust the other record only when its legacy bytes exactly explain the Mac mojibake.
		encoded, err := codec.NewEncoder().Bytes([]byte(other.Value))
		if err == nil && bytes.Equal(raw, encoded) {
			return true
		}
	}
	return false
}

func shadowedUnicodeName(name NameRecord, records []NameRecord) bool {
	ascii := asciiNameMatchesMetadata(name.Value, records)
	for _, other := range records {
		if other.ID != name.ID || !other.Decoded || other.Value == "" || other.Platform != 3 || other.Language != name.Language {
			continue
		}
		codec := legacyNameEncoding(other)
		if codec == nil {
			continue
		}
		if ascii {
			// The ASCII bytes also identify the manufacturer in the font's metadata.
			return true
		}
		raw, err := codec.NewEncoder().Bytes([]byte(other.Value))
		if err != nil {
			continue
		}
		for _, wrongCodec := range []encoding.Encoding{simplifiedchinese.GBK, traditionalchinese.Big5} {
			if wrongCodec == codec {
				continue
			}
			wrong, ok := decodeLegacyName(raw, wrongCodec)
			// Some converters also appended one or two stray characters to this wrong conversion.
			if ok && wrong != other.Value && utf8.RuneCountInString(wrong) >= 4 &&
				strings.HasPrefix(name.Value, wrong) && utf8.RuneCountInString(name.Value)-utf8.RuneCountInString(wrong) <= 2 {
				return true
			}
		}
	}
	return false
}

func asciiNameMatchesMetadata(value string, records []NameRecord) bool {
	var raw strings.Builder
	for _, r := range value {
		if r>>8 < 0x20 || r>>8 > 0x7E || r&0xFF < 0x20 || r&0xFF > 0x7E {
			return false
		}
		raw.WriteByte(byte(r >> 8))
		raw.WriteByte(byte(r))
	}
	brand, _, found := strings.Cut(raw.String(), "-")
	if !found || len(brand) < 4 {
		return false
	}
	for _, c := range brand {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			return false
		}
	}
	for _, other := range records {
		if other.Decoded && (other.ID == 0 || other.ID == 8) && strings.Contains(strings.ToLower(other.Value), strings.ToLower(brand)) {
			return true
		}
	}
	return false
}
