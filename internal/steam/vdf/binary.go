// Package vdf reads and writes Steam's binary KeyValues format, the one
// shortcuts.vdf uses. Maps keep their key order and unknown value types
// round-trip unchanged, so rewriting a file Steam owns loses nothing.
package vdf

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Binary KeyValues type tags.
const (
	typeMap     byte = 0x00
	typeString  byte = 0x01
	typeInt32   byte = 0x02
	typeFloat32 byte = 0x03
	typePointer byte = 0x04
	typeWString byte = 0x05
	typeColor   byte = 0x06
	typeUint64  byte = 0x07
	typeEnd     byte = 0x08
	typeInt64   byte = 0x0a
)

// Field is one key and its value: *Map, string, int32, uint64, int64,
// float32, or Raw for the rarely used fixed-width types.
type Field struct {
	Key   string
	Value any
}

// Raw keeps a pointer or color value's type tag and 4 bytes.
type Raw struct {
	Type byte
	Data [4]byte
}

// Map is an ordered list of fields.
type Map struct {
	Fields []Field
}

// Get returns the value under key (case insensitive, like Steam).
func (m *Map) Get(key string) (any, bool) {
	if i := m.index(key); i >= 0 {
		return m.Fields[i].Value, true
	}
	return nil, false
}

// Set replaces the value under key, or appends it.
func (m *Map) Set(key string, value any) {
	if i := m.index(key); i >= 0 {
		m.Fields[i].Value = value
		return
	}
	m.Fields = append(m.Fields, Field{Key: key, Value: value})
}

// Delete removes key.
func (m *Map) Delete(key string) {
	if i := m.index(key); i >= 0 {
		m.Fields = append(m.Fields[:i], m.Fields[i+1:]...)
	}
}

// String returns a string field, or "".
func (m *Map) String(key string) string {
	v, _ := m.Get(key)
	s, _ := v.(string)
	return s
}

// Int returns an int32 field, or 0.
func (m *Map) Int(key string) int32 {
	v, _ := m.Get(key)
	n, _ := v.(int32)
	return n
}

// Child returns a nested map, or nil.
func (m *Map) Child(key string) *Map {
	v, _ := m.Get(key)
	c, _ := v.(*Map)
	return c
}

func (m *Map) index(key string) int {
	for i, f := range m.Fields {
		if equalFold(f.Key, key) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool { return len(a) == len(b) && bytes.EqualFold([]byte(a), []byte(b)) }

// Parse reads a binary KeyValues document.
func Parse(r io.Reader) (*Map, error) {
	br := bufio.NewReader(r)
	root, err := parseMap(br, 0)
	if err != nil {
		return nil, err
	}
	return root, nil
}

// maxDepth bounds nesting in a corrupt or hostile file.
const maxDepth = 64

func parseMap(r *bufio.Reader, depth int) (*Map, error) {
	if depth > maxDepth {
		return nil, errors.New("vdf: nesting too deep")
	}
	m := &Map{}
	for {
		t, err := r.ReadByte()
		if err == io.EOF && depth == 0 {
			return m, nil
		}
		if err != nil {
			return nil, fmt.Errorf("vdf: %w", io.ErrUnexpectedEOF)
		}
		if t == typeEnd {
			return m, nil
		}
		key, err := readCString(r)
		if err != nil {
			return nil, err
		}
		var v any
		switch t {
		case typeMap:
			v, err = parseMap(r, depth+1)
		case typeString:
			v, err = readCString(r)
		case typeInt32:
			var n int32
			err = binary.Read(r, binary.LittleEndian, &n)
			v = n
		case typeFloat32:
			var n uint32
			err = binary.Read(r, binary.LittleEndian, &n)
			v = math.Float32frombits(n)
		case typePointer, typeColor:
			raw := Raw{Type: t}
			_, err = io.ReadFull(r, raw.Data[:])
			v = raw
		case typeUint64:
			var n uint64
			err = binary.Read(r, binary.LittleEndian, &n)
			v = n
		case typeInt64:
			var n int64
			err = binary.Read(r, binary.LittleEndian, &n)
			v = n
		default:
			return nil, fmt.Errorf("vdf: unsupported type 0x%02x for key %q", t, key)
		}
		if err != nil {
			return nil, fmt.Errorf("vdf: key %q: %w", key, err)
		}
		m.Fields = append(m.Fields, Field{Key: key, Value: v})
	}
}

func readCString(r *bufio.Reader) (string, error) {
	s, err := r.ReadString(0)
	if err != nil {
		return "", fmt.Errorf("vdf: unterminated string: %w", io.ErrUnexpectedEOF)
	}
	return s[:len(s)-1], nil
}

// Marshal encodes m as a binary KeyValues document. The root map's own end
// marker is written, as Steam does for shortcuts.vdf.
func Marshal(m *Map) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeMap(&buf, m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeMap(w *bytes.Buffer, m *Map) error {
	for _, f := range m.Fields {
		if bytes.IndexByte([]byte(f.Key), 0) >= 0 {
			return fmt.Errorf("vdf: key %q contains NUL", f.Key)
		}
		switch v := f.Value.(type) {
		case *Map:
			w.WriteByte(typeMap)
			writeCString(w, f.Key)
			if err := writeMap(w, v); err != nil {
				return err
			}
		case string:
			if bytes.IndexByte([]byte(v), 0) >= 0 {
				return fmt.Errorf("vdf: value of %q contains NUL", f.Key)
			}
			w.WriteByte(typeString)
			writeCString(w, f.Key)
			writeCString(w, v)
		case int32:
			w.WriteByte(typeInt32)
			writeCString(w, f.Key)
			_ = binary.Write(w, binary.LittleEndian, v)
		case float32:
			w.WriteByte(typeFloat32)
			writeCString(w, f.Key)
			_ = binary.Write(w, binary.LittleEndian, math.Float32bits(v))
		case Raw:
			w.WriteByte(v.Type)
			writeCString(w, f.Key)
			w.Write(v.Data[:])
		case uint64:
			w.WriteByte(typeUint64)
			writeCString(w, f.Key)
			_ = binary.Write(w, binary.LittleEndian, v)
		case int64:
			w.WriteByte(typeInt64)
			writeCString(w, f.Key)
			_ = binary.Write(w, binary.LittleEndian, v)
		default:
			return fmt.Errorf("vdf: unsupported value type %T for key %q", f.Value, f.Key)
		}
	}
	w.WriteByte(typeEnd)
	return nil
}

func writeCString(w *bytes.Buffer, s string) {
	w.WriteString(s)
	w.WriteByte(0)
}
