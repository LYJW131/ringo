package applemusic

import (
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
)

// omap is a map that remembers the order its keys were set in, as a
// JavaScript Map does: setting a key again moves it to the end, and the
// oldest is the first to go when there are too many. Its JSON is an object
// in that order, so what's kept on disk reads back the same. It isn't safe
// for use by several goroutines: its owner's lock covers it.
type omap[V any] struct {
	order *list.List // of string
	items map[string]omapItem[V]
}

type omapItem[V any] struct {
	value V
	at    *list.Element
}

func newOmap[V any]() *omap[V] {
	return &omap[V]{order: list.New(), items: map[string]omapItem[V]{}}
}

func (m *omap[V]) get(k string) (V, bool) {
	it, ok := m.items[k]
	return it.value, ok
}

func (m *omap[V]) has(k string) bool {
	_, ok := m.items[k]
	return ok
}

// set puts k last, with v.
func (m *omap[V]) set(k string, v V) {
	m.delete(k)
	m.items[k] = omapItem[V]{v, m.order.PushBack(k)}
}

func (m *omap[V]) delete(k string) {
	if it, ok := m.items[k]; ok {
		m.order.Remove(it.at)
		delete(m.items, k)
	}
}

func (m *omap[V]) len() int { return len(m.items) }

// trim drops the oldest keys until there are no more than most, answering them.
func (m *omap[V]) trim(most int) []string {
	var gone []string
	for m.len() > most {
		k := m.order.Front().Value.(string)
		m.delete(k)
		gone = append(gone, k)
	}
	return gone
}

func (m *omap[V]) keys() []string {
	out := make([]string, 0, m.len())
	for e := m.order.Front(); e != nil; e = e.Next() {
		out = append(out, e.Value.(string))
	}
	return out
}

func (m *omap[V]) clear() {
	m.order.Init()
	m.items = map[string]omapItem[V]{}
}

func (m *omap[V]) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range m.keys() {
		if i > 0 {
			b.WriteByte(',')
		}
		kj, _ := json.Marshal(k)
		vj, err := json.Marshal(m.items[k].value)
		if err != nil {
			return nil, err
		}
		b.Write(kj)
		b.WriteByte(':')
		b.Write(vj)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (m *omap[V]) UnmarshalJSON(data []byte) error {
	if m.items == nil {
		*m = *newOmap[V]()
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return fmt.Errorf("not an object")
	}
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return err
		}
		k, _ := t.(string)
		var v V
		if err := d.Decode(&v); err != nil {
			return err
		}
		m.set(k, v)
	}
	return nil
}
