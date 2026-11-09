package widevine

import (
	"encoding/binary"
	"errors"
)

// Just enough of protocol buffers for Widevine's messages, without their
// schema: fields by number.

// value is one occurrence of a field: a number for a varint, bytes for the
// rest.
type value struct {
	num   uint64
	bytes []byte
	isNum bool
}

// fields is a message's fields, each a list of its values.
type fields map[int][]value

// field is one [number, value] pair to encode: a uint64 (or int) is a
// varint, a []byte or string is length-delimited.
type field struct {
	num int
	val any
}

func appendVarint(b []byte, v uint64) []byte {
	return binary.AppendUvarint(b, v)
}

// encode builds a message from fields in the order given.
func encode(fs ...field) []byte {
	var out []byte
	for _, f := range fs {
		switch v := f.val.(type) {
		case int:
			out = appendVarint(appendVarint(out, uint64(f.num)<<3), uint64(v))
		case uint64:
			out = appendVarint(appendVarint(out, uint64(f.num)<<3), v)
		case []byte:
			out = appendVarint(appendVarint(out, uint64(f.num)<<3|2), uint64(len(v)))
			out = append(out, v...)
		case string:
			out = appendVarint(appendVarint(out, uint64(f.num)<<3|2), uint64(len(v)))
			out = append(out, v...)
		default:
			panic("widevine: encode: unsupported field value")
		}
	}
	return out
}

var errTruncated = errors.New("widevine: protobuf: truncated message")

func decode(b []byte) (fields, error) {
	out := fields{}
	for at := 0; at < len(b); {
		tag, n := binary.Uvarint(b[at:])
		if n <= 0 {
			return nil, errTruncated
		}
		at += n
		num := int(tag >> 3)
		var v value
		take := func(size uint64) error {
			if uint64(len(b)-at) < size {
				return errTruncated
			}
			v = value{bytes: b[at : at+int(size)]}
			at += int(size)
			return nil
		}
		var err error
		switch tag & 7 {
		case 0:
			x, n := binary.Uvarint(b[at:])
			if n <= 0 {
				return nil, errTruncated
			}
			at += n
			v = value{num: x, isNum: true}
		case 1:
			err = take(8)
		case 2:
			size, n := binary.Uvarint(b[at:])
			if n <= 0 {
				return nil, errTruncated
			}
			at += n
			err = take(size)
		case 5:
			err = take(4)
		default:
			return nil, errors.New("widevine: protobuf: unknown wire type")
		}
		if err != nil {
			return nil, err
		}
		out[num] = append(out[num], v)
	}
	return out, nil
}

// numberOf is the field's first value, if it's a number.
func (f fields) numberOf(num int) (uint64, bool) {
	if vs := f[num]; len(vs) > 0 && vs[0].isNum {
		return vs[0].num, true
	}
	return 0, false
}

// bytesOf is the field's first value, if it's bytes.
func (f fields) bytesOf(num int) ([]byte, bool) {
	if vs := f[num]; len(vs) > 0 && !vs[0].isNum {
		return vs[0].bytes, true
	}
	return nil, false
}

// allBytesOf is all of the field's values that are bytes.
func (f fields) allBytesOf(num int) [][]byte {
	var out [][]byte
	for _, v := range f[num] {
		if !v.isNum {
			out = append(out, v.bytes)
		}
	}
	return out
}
