package djson

// Taken from https://github.com/segmentio/encoding

import (
	"bytes"
	"math/bits"
	"unicode/utf8"
	"unsafe"
)

const (
	lsb = 0x0101010101010101
	msb = 0x8080808080808080
)

// escapeIndex finds the index of the first char in `s` that requires escaping.
// A char requires escaping if it's outside of the range of [0x20, 0x7F] or if
// it includes a double quote or backslash. If the escapeHTML mode is enabled,
// the chars <, > and & also require escaping. If no chars in `s` require
// escaping, the return value is -1.
func escapeIndex(s string, escapeHTML bool) int {
	chunks := stringToUint64(s)
	for _, n := range chunks {
		// combine masks before checking for the MSB of each byte. We include
		// `n` in the mask to check whether any of the *input* byte MSBs were
		// set (i.e. the byte was outside the ASCII range).
		mask := n | below(n, 0x20) | contains(n, '"') | contains(n, '\\')
		if escapeHTML {
			mask |= contains(n, '<') | contains(n, '>') | contains(n, '&')
		}
		if (mask & msb) != 0 {
			return bits.TrailingZeros64(mask&msb) / 8
		}
	}

	for i := len(chunks) * 8; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7f || c == '"' || c == '\\' || (escapeHTML && (c == '<' || c == '>' || c == '&')) {
			return i
		}
	}

	return -1
}

// below return a mask that can be used to determine if any of the bytes
// in `n` are below `b`. If a byte's MSB is set in the mask then that byte was
// below `b`. The result is only valid if `b`, and each byte in `n`, is below
// 0x80.
func below(n uint64, b byte) uint64 {
	return n - expand(b)
}

// contains returns a mask that can be used to determine if any of the
// bytes in `n` are equal to `b`. If a byte's MSB is set in the mask then
// that byte is equal to `b`. The result is only valid if `b`, and each
// byte in `n`, is below 0x80.
func contains(n uint64, b byte) uint64 {
	return (n ^ expand(b)) - lsb
}

// expand puts the specified byte into each of the 8 bytes of a uint64.
func expand(b byte) uint64 {
	return lsb * uint64(b)
}

type sliceHeader struct {
	Data unsafe.Pointer
	Len  int
	Cap  int
}

func stringToUint64(s string) []uint64 {
	return *(*[]uint64)(unsafe.Pointer(&sliceHeader{
		Data: *(*unsafe.Pointer)(unsafe.Pointer(&s)),
		Len:  len(s) / 8,
		Cap:  len(s) / 8,
	}))
}

const gHex = "0123456789abcdef"

func encodeString(b *bytes.Buffer, s string) {

	if len(s) == 0 {
		b.WriteString(`""`)
		return
	}
	i := 0
	j := 0

	escapeHTML := true

	b.WriteByte('"')

	if len(s) >= 8 {
		if j = escapeIndex(s, escapeHTML); j < 0 {
			b.WriteString(s)
			b.WriteByte('"')
			return
		}
	}

	for j < len(s) {
		c := s[j]

		if c >= 0x20 && c <= 0x7f && c != '\\' && c != '"' && (!escapeHTML || (c != '<' && c != '>' && c != '&')) {
			// fast path: most of the time, printable ascii characters are used
			j++
			continue
		}

		switch c {
		case '\\', '"':
			b.WriteString(s[i:j])
			b.WriteByte('\\')
			b.WriteByte(c)
			i = j + 1
			j = j + 1
			continue

		case '\n':
			b.WriteString(s[i:j])
			b.WriteString(`\n`)
			i = j + 1
			j = j + 1
			continue

		case '\r':
			b.WriteString(s[i:j])
			b.WriteString(`\r`)
			i = j + 1
			j = j + 1
			continue

		case '\t':
			b.WriteString(s[i:j])
			b.WriteString(`\t`)
			i = j + 1
			j = j + 1
			continue

		case '<', '>', '&':
			b.WriteString(s[i:j])
			b.WriteString(`\u00`)
			b.WriteByte(gHex[c>>4])
			b.WriteByte(gHex[c&0xF])
			i = j + 1
			j = j + 1
			continue
		}

		// This encodes bytes < 0x20 except for \t, \n and \r.
		if c < 0x20 {
			b.WriteString(s[i:j])
			b.WriteString(`\u00`)
			b.WriteByte(gHex[c>>4])
			b.WriteByte(gHex[c&0xF])
			i = j + 1
			j = j + 1
			continue
		}

		r, size := utf8.DecodeRuneInString(s[j:])

		if r == utf8.RuneError && size == 1 {
			b.WriteString(s[i:j])
			b.WriteString(`\ufffd`)
			i = j + size
			j = j + size
			continue
		}

		switch r {
		case '\u2028', '\u2029':
			// U+2028 is LINE SEPARATOR.
			// U+2029 is PARAGRAPH SEPARATOR.
			// They are both technically valid characters in JSON strings,
			// but don't work in JSONP, which has to be evaluated as JavaScript,
			// and can lead to security holes there. It is valid JSON to
			// escape them, so we do so unconditionally.
			// See http://timelessrepo.com/json-isnt-a-javascript-subset for discussion.
			b.WriteString(s[i:j])
			b.WriteString(`\u202`)
			b.WriteByte(gHex[r&0xF])
			i = j + size
			j = j + size
			continue
		}

		j += size
	}

	b.WriteString(s[i:])
	b.WriteByte('"')
}
