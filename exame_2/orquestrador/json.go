package main

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"strings"
)

// codificarJSON é json.MarshalIndent com uma diferença: NaN e ±Inf viram
// null. O encoding/json recusa esses valores, mas uma razão indefinida (por
// exemplo, amplificação num intervalo sem pedidos) é um resultado legítimo e
// não pode impedir a gravação do manifesto. A ordem dos campos é preservada.
func codificarJSON(v any) ([]byte, error) {
	var bruto bytes.Buffer
	if err := escrever(&bruto, reflect.ValueOf(v)); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, bruto.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func escrever(b *bytes.Buffer, v reflect.Value) error {
	if !v.IsValid() {
		b.WriteString("null")
		return nil
	}
	// Tipos que sabem se codificar (time.Time, json.RawMessage...) ficam com
	// o encoding/json.
	if v.CanInterface() {
		if _, ok := v.Interface().(json.Marshaler); ok {
			return padrao(b, v)
		}
	}
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			b.WriteString("null")
			return nil
		}
		return padrao(b, v)
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		return escrever(b, v.Elem())
	case reflect.Struct:
		b.WriteByte('{')
		primeiro := true
		if err := camposStruct(b, v, &primeiro); err != nil {
			return err
		}
		b.WriteByte('}')
		return nil
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			b.WriteString("null")
			return nil
		}
		b.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := escrever(b, v.Index(i)); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	case reflect.Map:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		chaves := v.MapKeys()
		sort.Slice(chaves, func(i, j int) bool { return chaves[i].String() < chaves[j].String() })
		b.WriteByte('{')
		for i, k := range chaves {
			if i > 0 {
				b.WriteByte(',')
			}
			nome, _ := json.Marshal(k.String())
			b.Write(nome)
			b.WriteByte(':')
			if err := escrever(b, v.MapIndex(k)); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	}
	return padrao(b, v)
}

func padrao(b *bytes.Buffer, v reflect.Value) error {
	enc, err := json.Marshal(v.Interface())
	if err != nil {
		return err
	}
	b.Write(enc)
	return nil
}

func camposStruct(b *bytes.Buffer, v reflect.Value, primeiro *bool) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		campo := t.Field(i)
		if !campo.IsExported() {
			continue
		}
		tag := campo.Tag.Get("json")
		if tag == "-" {
			continue
		}
		nome, opcoes, _ := strings.Cut(tag, ",")
		// Struct embutida sem nome próprio: os campos sobem um nível.
		if campo.Anonymous && nome == "" && campo.Type.Kind() == reflect.Struct {
			if err := camposStruct(b, v.Field(i), primeiro); err != nil {
				return err
			}
			continue
		}
		if nome == "" {
			nome = campo.Name
		}
		if strings.Contains(opcoes, "omitempty") && vazio(v.Field(i)) {
			continue
		}
		if !*primeiro {
			b.WriteByte(',')
		}
		*primeiro = false
		chave, _ := json.Marshal(nome)
		b.Write(chave)
		b.WriteByte(':')
		if err := escrever(b, v.Field(i)); err != nil {
			return err
		}
	}
	return nil
}

func vazio(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Slice, reflect.Map, reflect.String, reflect.Array:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	}
	return v.IsZero()
}
