package sse

import (
	"context"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStream(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	resp := httptest.NewRecorder()
	ch := make(chan string)
	fire := func(v string) {
		ch <- v
	}
	go func() {
		err := Stream(context.TODO(), resp, ch, InitialValues("0", "0"))
		assert.Nil(t, err)
		wg.Done()
	}()
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, "text/event-stream", resp.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", resp.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", resp.Header().Get("Connection"))
	assert.Equal(t, "data: \"0\"\n\ndata: \"0\"\n\n", resp.Body.String())
	fire("1")
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, "data: \"0\"\n\ndata: \"0\"\n\ndata: \"1\"\n\n", resp.Body.String())
	close(ch)
	wg.Wait()
}

func TestFormater(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	resp := httptest.NewRecorder()
	ch := make(chan string)
	fire := func(v string) {
		ch <- v
	}
	go func() {
		err := Stream(context.TODO(), resp, ch, Formater(func(w io.Writer, t string) error {
			_, err := io.WriteString(w, t)
			return err
		}))
		assert.Nil(t, err)
		wg.Done()
	}()
	fire("1")
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, "data: 1\n\n", resp.Body.String())
	close(ch)
	wg.Wait()
}
