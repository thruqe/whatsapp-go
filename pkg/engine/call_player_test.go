package whatsmeow

import (
	"io"
	"testing"
)

type dummyAudioSource struct {
	frames [][]float32
	closed bool
}

func (d *dummyAudioSource) ReadFrame() ([]float32, error) {
	if len(d.frames) == 0 {
		return nil, io.EOF
	}
	f := d.frames[0]
	d.frames = d.frames[1:]
	return f, nil
}

func (d *dummyAudioSource) Close() error {
	d.closed = true
	return nil
}

func TestPlayerOnStartAndOnFinish(t *testing.T) {
	player := NewPlayer()
	if player.Started() {
		t.Fatalf("new player should not be started")
	}

	src := &dummyAudioSource{
		frames: [][]float32{
			make([]float32, FrameSamples),
			make([]float32, FrameSamples),
		},
	}

	var startFired, finishFired bool
	player.OnStart(func() {
		startFired = true
	})
	player.OnFinish(func() {
		finishFired = true
	})

	player.Play(src)
	if player.Started() {
		t.Fatalf("player should not be started until first frame is read")
	}

	// Pull first frame
	frame1 := player.nextFrame()
	if frame1 == nil {
		t.Fatalf("expected non-nil first frame")
	}
	if !player.Started() {
		t.Fatalf("player.Started() should be true after first frame")
	}
	if !startFired {
		t.Fatalf("OnStart callback was not invoked")
	}

	// Pull second frame
	frame2 := player.nextFrame()
	if frame2 == nil {
		t.Fatalf("expected non-nil second frame")
	}
	if finishFired {
		t.Fatalf("finish callback fired too early")
	}

	// Pull EOF
	frame3 := player.nextFrame()
	if frame3 != nil {
		t.Fatalf("expected nil frame on EOF")
	}
	if !finishFired {
		t.Fatalf("OnFinish callback was not invoked on EOF")
	}
	if !src.closed {
		t.Fatalf("source was not closed on EOF")
	}
	if player.Started() {
		t.Fatalf("player.Started() should be false after finishing")
	}
}

func TestCallOnMediaStart(t *testing.T) {
	c := &Call{}
	if c.MediaStarted() {
		t.Fatalf("new call should not have media started")
	}

	var mediaStartedFired bool
	c.OnMediaStart(func() {
		mediaStartedFired = true
	})

	c.markMediaStarted()
	if !c.MediaStarted() {
		t.Fatalf("MediaStarted should be true after markMediaStarted")
	}
	if !mediaStartedFired {
		t.Fatalf("OnMediaStart callback not fired")
	}

	// Registering after it has already started should fire immediately
	var secondCallbackFired bool
	c.OnMediaStart(func() {
		secondCallbackFired = true
	})
	if !secondCallbackFired {
		t.Fatalf("late registered callback should be invoked immediately")
	}
}
