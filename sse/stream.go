package sse

import (
	"bytes"
	"context"
	"errors"
	"net/http"
)

func Stream[T any](ctx context.Context, w http.ResponseWriter, source <-chan T, opts ...OptionFunc[T]) error {
	var options Options[T]
	for _, opt := range opts {
		opt(&options)
	}
	if options.Formater == nil {
		options.Formater = jsonFormat[T]
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("streaming unsupported")
	}

	if options.Initial != nil {
		for _, v := range options.Initial {
			err := send(w, flusher, v, options.Formater)
			if err != nil {
				return err
			}
		}
	}

	for {
		select {
		case v, ok := <-source:
			if !ok {
				return nil
			}
			err := send(w, flusher, v, options.Formater)
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func send[T any](w http.ResponseWriter, flusher http.Flusher, data T, formater DataFormater[T]) error {
	buf := bytes.NewBuffer([]byte("data: "))
	err := formater(buf, data)
	if err != nil {
		return err
	}
	buf.WriteString("\n\n")
	_, err = w.Write(buf.Bytes())
	if err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
