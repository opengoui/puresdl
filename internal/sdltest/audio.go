package sdltest

import (
	"testing"
	"unsafe"

	"github.com/opengoui/puresdl/sdl"
)

// OpenPlayback opens a stream on the default playback device, which is the
// dummy driver's device. The device starts paused (SDL's default for
// sdl.OpenAudioDeviceStream), so queued data stays queued and [Queued] is
// deterministic until the code under test calls sdl.ResumeAudioStreamDevice.
// SDL audio must be initialized (see [Setup] with sdl.INIT_AUDIO). The stream
// is destroyed on cleanup.
func OpenPlayback(t testing.TB, spec sdl.AudioSpec) *sdl.AudioStream {
	t.Helper()
	stream := Call(t, func() *sdl.AudioStream {
		return sdl.OpenAudioDeviceStream(sdl.AUDIO_DEVICE_DEFAULT_PLAYBACK, &spec, sdl.NoAudioStreamCallback, nil)
	})
	if stream == nil {
		t.Fatalf("SDL_OpenAudioDeviceStream: %s", sdl.GetError())
	}
	t.Cleanup(func() { OnMain(t, func() { sdl.DestroyAudioStream(stream) }) })
	return stream
}

// Queued returns the bytes waiting in the stream that were put by the code
// under test and have not been consumed by the device yet.
func Queued(t testing.TB, stream *sdl.AudioStream) int {
	t.Helper()
	n := Call(t, func() int32 { return sdl.GetAudioStreamQueued(stream) })
	if n < 0 {
		t.Fatalf("SDL_GetAudioStreamQueued: %s", sdl.GetError())
	}
	return int(n)
}

// AudioProbe is a stream that is not bound to any device, so everything put
// into it stays readable. Inject it into code under test in place of a device
// stream, then inspect what was produced.
type AudioProbe struct {
	t      testing.TB
	Stream *sdl.AudioStream
}

// NewAudioProbe creates a probe whose input and output formats are both spec,
// so bytes read back equal bytes put in. It is destroyed on cleanup.
func NewAudioProbe(t testing.TB, spec sdl.AudioSpec) *AudioProbe {
	t.Helper()
	p := &AudioProbe{t: t}
	p.Stream = Call(t, func() *sdl.AudioStream { return sdl.CreateAudioStream(&spec, &spec) })
	if p.Stream == nil {
		t.Fatalf("SDL_CreateAudioStream: %s", sdl.GetError())
	}
	t.Cleanup(func() { OnMain(t, func() { sdl.DestroyAudioStream(p.Stream) }) })
	return p
}

// Queued returns the bytes put into the probe and not yet read.
func (p *AudioProbe) Queued() int {
	p.t.Helper()
	return Queued(p.t, p.Stream)
}

// ReadAll flushes the probe and removes and returns everything queued.
func (p *AudioProbe) ReadAll() []byte {
	p.t.Helper()
	return Call(p.t, func() []byte {
		sdl.FlushAudioStream(p.Stream)
		buf := make([]byte, sdl.GetAudioStreamAvailable(p.Stream))
		if len(buf) == 0 {
			return buf
		}
		n := sdl.GetAudioStreamData(p.Stream, unsafe.Pointer(&buf[0]), int32(len(buf)))
		return buf[:max(n, 0)]
	})
}
