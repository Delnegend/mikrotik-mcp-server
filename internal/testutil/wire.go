package testutil

import "errors"

// DecodeSentences splits raw RouterOS wire bytes into complete sentences.
func DecodeSentences(raw []byte) ([][]string, error) {
	var sentences [][]string
	pos := 0
	for pos < len(raw) {
		var words []string
		for {
			length, n, err := DecodeWordLength(raw[pos:])
			if err != nil {
				return nil, err
			}
			pos += n
			if length == 0 {
				break
			}
			words = append(words, string(raw[pos:pos+length]))
			pos += length
		}
		sentences = append(sentences, words)
	}
	return sentences, nil
}

// DecodeWordLength decodes a RouterOS variable-length word prefix, returning
// the word length and the number of bytes consumed.
func DecodeWordLength(b []byte) (int, int, error) {
	if len(b) < 1 {
		return 0, 0, errors.New("truncated length prefix")
	}
	v := b[0]
	switch {
	case v&0x80 == 0:
		return int(v), 1, nil
	case v&0xC0 == 0x80:
		if len(b) < 2 {
			return 0, 0, errors.New("truncated length prefix")
		}
		return int(uint16(v&0x3F)<<8 | uint16(b[1])), 2, nil
	default:
		return 0, 0, errors.New("unsupported length prefix")
	}
}
