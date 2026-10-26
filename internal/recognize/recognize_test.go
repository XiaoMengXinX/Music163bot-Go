package recognize

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testAudio() []byte {
	const sampleRate = 8000
	const samples = sampleRate * 6
	audio := make([]byte, 44+samples*2)
	copy(audio, "RIFF")
	binary.LittleEndian.PutUint32(audio[4:], uint32(len(audio)-8))
	copy(audio[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(audio[16:], 16)
	binary.LittleEndian.PutUint16(audio[20:], 1)
	binary.LittleEndian.PutUint16(audio[22:], 1)
	binary.LittleEndian.PutUint32(audio[24:], sampleRate)
	binary.LittleEndian.PutUint32(audio[28:], sampleRate*2)
	binary.LittleEndian.PutUint16(audio[32:], 2)
	binary.LittleEndian.PutUint16(audio[34:], 16)
	copy(audio[36:], "data")
	binary.LittleEndian.PutUint32(audio[40:], samples*2)
	for i := range samples {
		sample := int16(16000 * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
		binary.LittleEndian.PutUint16(audio[44+i*2:], uint16(sample))
	}
	return audio
}

func TestRecognize(t *testing.T) {
	audio := testAudio()
	for _, tt := range []struct {
		name     string
		body     string
		wantID   int
		wantErr  error
		otherErr bool
	}{
		{name: "first song", body: `{"code":200,"data":{"result":[{"song":{"id":123}},{"song":{"id":456}}]}}`, wantID: 123},
		{name: "no match", body: `{"code":200,"data":{"result":[]}}`, wantErr: ErrNoResult},
		{name: "null data", body: `{"code":200,"data":null}`, wantErr: ErrNoResult},
		{name: "missing data", body: `{"code":200}`, wantErr: ErrNoResult},
		{name: "missing song", body: `{"code":200,"data":{"result":[{}]}}`, wantErr: ErrNoResult},
		{name: "API failure", body: `{"code":500,"data":null}`, otherErr: true},
		{name: "invalid JSON", body: `not json`, otherErr: true},
		{name: "invalid result", body: `{"code":200,"data":{"result":"invalid"}}`, otherErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			c := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				called = true
				if r.URL.Host != "interface.music.163.com" || r.URL.Path != "/api/music/audio/match" || r.Method != http.MethodPost {
					t.Errorf("unexpected recognition request: %s %s", r.Method, r.URL)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if r.Form.Get("rawdata") == "" {
					t.Fatal("missing audio fingerprint")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})})
			id, err := c.Recognize(t.Context(), audio)
			if !called {
				t.Fatal("audio input did not reach audio matching")
			}
			if id != tt.wantID {
				t.Errorf("song ID = %d, want %d", id, tt.wantID)
			}
			if tt.otherErr {
				if err == nil || errors.Is(err, ErrNoResult) {
					t.Fatalf("expected recognition error, got %v", err)
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRecognizeInvalidAudio(t *testing.T) {
	c := New(&http.Client{})
	c.api.HTTP.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid audio must not reach audio matching")
		return nil, nil
	})
	if _, err := c.Recognize(t.Context(), []byte("not audio")); err == nil {
		t.Fatal("expected decoding error")
	}
}

func TestRecognizeCancellation(t *testing.T) {
	audio := testAudio()
	c := New(&http.Client{})
	c.api.HTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.Recognize(ctx, audio); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
