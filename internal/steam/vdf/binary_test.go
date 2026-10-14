package vdf

import (
	"bytes"
	"testing"
)

// A shortcuts.vdf as Steam writes it: one shortcut with a tag.
var sample = []byte("\x00shortcuts\x00" +
	"\x000\x00" +
	"\x02appid\x00\x01\x02\x03\x84" +
	"\x01AppName\x00Game\x00" +
	"\x01Exe\x00\"/bin/game\"\x00" +
	"\x02LastPlayTime\x00\x00\x00\x00\x00" +
	"\x00tags\x00\x010\x00favorite\x00\x08" +
	"\x08" +
	"\x08" +
	"\x08")

func TestRoundTrip(t *testing.T) {
	m, err := Parse(bytes.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	sc := m.Child("shortcuts").Child("0")
	if sc == nil || sc.String("appname") != "Game" || uint32(sc.Int("appid")) != 0x84030201 {
		t.Fatalf("parsed %+v", sc)
	}
	if sc.Child("tags").String("0") != "favorite" {
		t.Fatal("tags lost")
	}
	out, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, sample) {
		t.Fatalf("round trip differs:\n%q\n%q", out, sample)
	}
}

func TestTruncated(t *testing.T) {
	if _, err := Parse(bytes.NewReader(sample[:20])); err == nil {
		t.Fatal("truncated file parsed")
	}
}
