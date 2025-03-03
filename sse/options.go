package sse

import (
	"io"

	"github.com/bytedance/sonic/encoder"
)

type DataFormater[T any] func(io.Writer, T) error

type Options[T any] struct {
	Initial  []T
	Formater DataFormater[T]
}

type OptionFunc[T any] func(opts *Options[T])

func InitialValues[T any](values ...T) OptionFunc[T] {
	return func(opts *Options[T]) {
		opts.Initial = values
	}
}

func Formater[T any](fn DataFormater[T]) OptionFunc[T] {
	return func(opts *Options[T]) {
		opts.Formater = fn
	}
}

func jsonFormat[T any](w io.Writer, v T) error {
	enc := encoder.NewStreamEncoder(w)
	enc.SetNoEncoderNewline(true)
	return enc.Encode(v)
}
