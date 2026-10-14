package store

import (
	"encoding/hex"
	"strings"
)

// npub encodes a hex public key as a NIP-19 npub, or returns "" for a
// malformed key.
func npub(pubkeyHex string) string {
	data, err := hex.DecodeString(pubkeyHex)
	if err != nil || len(data) != 32 {
		return ""
	}
	return bech32Encode("npub", convertBits(data, 8, 5))
}

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

func bech32Polymod(values []byte) uint32 {
	gen := [5]uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := uint32(1)
	for _, v := range values {
		top := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ uint32(v)
		for i := range 5 {
			if (top>>i)&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

func bech32Encode(hrp string, data []byte) string {
	values := make([]byte, 0, len(hrp)*2+1+len(data)+6)
	for _, c := range hrp {
		values = append(values, byte(c)>>5)
	}
	values = append(values, 0)
	for _, c := range hrp {
		values = append(values, byte(c)&31)
	}
	values = append(values, data...)
	values = append(values, 0, 0, 0, 0, 0, 0)
	mod := bech32Polymod(values) ^ 1

	var b strings.Builder
	b.WriteString(hrp)
	b.WriteByte('1')
	for _, d := range data {
		b.WriteByte(bech32Charset[d])
	}
	for i := range 6 {
		b.WriteByte(bech32Charset[(mod>>(5*(5-i)))&31])
	}
	return b.String()
}

// convertBits regroups bytes of from bits into groups of to bits, padding
// the last group.
func convertBits(data []byte, from, to uint) []byte {
	var acc, bits uint
	var out []byte
	maxv := uint(1)<<to - 1
	for _, v := range data {
		acc = acc<<from | uint(v)
		bits += from
		for bits >= to {
			bits -= to
			out = append(out, byte(acc>>bits&maxv))
		}
	}
	if bits > 0 {
		out = append(out, byte(acc<<(to-bits)&maxv))
	}
	return out
}
