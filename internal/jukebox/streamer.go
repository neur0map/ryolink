package jukebox

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Streamer fetches MP3 data from track URLs and sends it to connected clients.
type Streamer struct {
	mu              sync.RWMutex
	cancel          context.CancelFunc
	currentTrack    *Track
	audioData       []byte    // complete MP3 for current track
	playStart       time.Time // when the current track started playing
	client          *http.Client
	onDurationKnown func(int)              // callback to update engine with actual duration
	onError         func()                 // callback when download fails (engine retries)
	trackSubs       map[chan struct{}]bool // web stream subscribers
}

// NewStreamer creates a new audio streamer.
func NewStreamer() *Streamer {
	return &Streamer{
		client: &http.Client{},
	}
}

// SetOnDurationKnown sets a callback invoked when actual track duration is known.
func (s *Streamer) SetOnDurationKnown(fn func(int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDurationKnown = fn
}

// SetOnError sets a callback invoked when a download fails.
func (s *Streamer) SetOnError(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onError = fn
}

// StreamTrack downloads the track and sends it to all connected clients.
func (s *Streamer) StreamTrack(track Track) {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.currentTrack = &track
	s.audioData = nil
	s.mu.Unlock()

	go s.downloadAndBroadcast(ctx, track)
}

// CurrentAudio returns the current track, its audio data, and when playback started.
func (s *Streamer) CurrentAudio() (*Track, []byte, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.currentTrack == nil || len(s.audioData) == 0 {
		return nil, nil, time.Time{}
	}
	audio := make([]byte, len(s.audioData))
	copy(audio, s.audioData)
	t := *s.currentTrack
	return &t, audio, s.playStart
}

// SubscribeTrackChange returns a channel that receives a signal when a new track starts playing.
// Call UnsubscribeTrackChange to clean up.
func (s *Streamer) SubscribeTrackChange() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan struct{}, 1)
	if s.trackSubs == nil {
		s.trackSubs = make(map[chan struct{}]bool)
	}
	s.trackSubs[ch] = true
	return ch
}

// UnsubscribeTrackChange removes a track change subscription.
func (s *Streamer) UnsubscribeTrackChange(ch chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.trackSubs, ch)
	close(ch)
}

// Stop cancels the current download.
func (s *Streamer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.currentTrack = nil
	s.audioData = nil
}

func (s *Streamer) downloadAndBroadcast(ctx context.Context, track Track) {
	if track.URL == "" {
		s.signalError()
		return
	}

	log.Printf("streamer: downloading %s", track.Title)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, track.URL, nil)
	if err != nil {
		log.Printf("streamer: request error: %v", err)
		s.signalError()
		return
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Printf("streamer: fetch error: %v", err)
		s.signalError()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("streamer: HTTP %d for %s", resp.StatusCode, track.Title)
		s.signalError()
		return
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Printf("streamer: read error: %v", err)
		s.signalError()
		return
	}

	if len(audio) < 1000 {
		log.Printf("streamer: audio too small (%d bytes), skipping %s", len(audio), track.Title)
		s.signalError()
		return
	}

	estimatedDuration := estimateMP3Duration(audio)
	log.Printf("streamer: downloaded %d bytes (~%ds)", len(audio), estimatedDuration)

	s.mu.Lock()
	if s.onDurationKnown != nil {
		go s.onDurationKnown(estimatedDuration)
	}

	// Store the audio data and mark playback start time
	s.audioData = audio
	s.playStart = time.Now()
	// Notify web stream subscribers
	for ch := range s.trackSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *Streamer) signalError() {
	s.mu.RLock()
	fn := s.onError
	s.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// estimateMP3Duration returns the duration in seconds.
// Uses ffprobe if available for accuracy, otherwise estimates from file size.
func estimateMP3Duration(data []byte) int {
	if _, err := exec.LookPath("ffprobe"); err == nil {
		if dur := ffprobeDuration(data); dur > 0 {
			return dur
		}
	}
	// Fallback: assume ~176kbps (typical for chillhop streams)
	dur := len(data) / 22000
	if dur < 10 {
		dur = 10
	}
	return dur
}

func ffprobeDuration(data []byte) int {
	tmp, err := os.CreateTemp("", "ryolink-probe-*.mp3")
	if err != nil {
		return 0
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		return 0
	}
	tmp.Close()

	out, err := exec.Command("ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		tmp.Name(),
	).Output()
	if err != nil {
		return 0
	}

	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return int(f)
}
