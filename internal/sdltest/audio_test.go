package sdltest_test

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/opengoui/puresdl/internal/sdltest"
	"github.com/opengoui/puresdl/sdl"
)

var s16mono = sdl.AudioSpec{Format: sdl.AUDIO_S16, Channels: 1, Freq: 8000}

func put(t *testing.T, s *sdl.AudioStream, data []byte) {
	t.Helper()
	ok := sdltest.Call(t, func() bool { return sdl.PutAudioStreamData(s, unsafe.Pointer(&data[0]), int32(len(data))) })
	if !ok {
		t.Fatalf("PutAudioStreamData: %s", sdl.GetError())
	}
}

func TestPlaybackStreamKeepsQueuedBytesWhilePaused(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_AUDIO)
	stream := sdltest.OpenPlayback(t, s16mono)
	if q := sdltest.Queued(t, stream); q != 0 {
		t.Fatalf("fresh stream has %d bytes queued", q)
	}
	put(t, stream, make([]byte, 1600))
	put(t, stream, make([]byte, 400))
	if q := sdltest.Queued(t, stream); q != 2000 {
		t.Fatalf("queued = %d, want 2000", q)
	}
}

func TestAudioProbeReturnsWhatWasPut(t *testing.T) {
	sdltest.Setup(t, sdl.INIT_AUDIO)
	p := sdltest.NewAudioProbe(t, s16mono)
	in := []byte{1, 0, 2, 0, 3, 0, 4, 0}
	put(t, p.Stream, in)
	if q := p.Queued(); q != len(in) {
		t.Fatalf("queued = %d, want %d", q, len(in))
	}
	if got := p.ReadAll(); !bytes.Equal(got, in) {
		t.Fatalf("ReadAll = %v, want %v", got, in)
	}
	if q := p.Queued(); q != 0 {
		t.Fatalf("queued after ReadAll = %d", q)
	}
}
