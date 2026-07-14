package neo

import (
	"encoding/json"
	"io"
)

var DefaultJSONCodec = &stdJsonCodec{}

type JSONCodec interface {
	Encode(v any) ([]byte, error)
	EncodeStream(v any, w io.Writer) error
	Decode(data []byte, v any) error
	DecodeStream(r io.Reader, v any) error
}

type stdJsonCodec struct{}

func (s *stdJsonCodec) Encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (s *stdJsonCodec) EncodeStream(v any, w io.Writer) error {
	return json.NewEncoder(w).Encode(v)
}

func (s *stdJsonCodec) Decode(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (s *stdJsonCodec) DecodeStream(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}
