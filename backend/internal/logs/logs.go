// Package logs writes app output to daily, size-capped files and fans it out to live viewers.
package logs

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxFileSize = 50 << 20
	tailLines   = 500
)

// Stream is an append-only log: a set of daily files plus an in-memory tail for live viewers.
type Stream struct {
	dir    string
	prefix string
	fixed  string

	mu      sync.Mutex
	file    *os.File
	day     string
	size    int64
	part    int
	partial []byte
	recent  []string
	subs    map[chan string]struct{}
}

// NewStream writes to <dir>/<prefix>-<date>.log, rotating daily and by size. Live viewers
// start with the end of the newest file, so a restart doesn't leave them with nothing.
func NewStream(dir, prefix string) *Stream {
	s := &Stream{dir: dir, prefix: prefix, subs: map[chan string]struct{}{}}
	s.recent = lastLines(dir, prefix)
	return s
}

// lastLines reads the last tailLines lines of the newest log file.
func lastLines(dir, prefix string) []string {
	days := Days(dir, prefix)
	if len(days) == 0 {
		return nil
	}
	matches, _ := filepath.Glob(filepath.Join(dir, prefix+"-"+days[0]+"*.log"))
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool { return partOf(matches[i]) < partOf(matches[j]) })
	lines, _ := TailFile(matches[len(matches)-1], tailLines)
	return lines
}

// NewFile writes to a single file with no rotation, used for deployment logs.
func NewFile(path string) *Stream {
	return &Stream{dir: filepath.Dir(path), fixed: path, subs: map[chan string]struct{}{}}
}

// Write splits output into lines, timestamps each one and stores it.
func (s *Stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := append(s.partial, p...)
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		s.writeLine(strings.TrimRight(string(data[:i]), "\r"))
		data = data[i+1:]
	}
	s.partial = append([]byte(nil), data...)
	if len(s.partial) > 64<<10 {
		s.writeLine(string(s.partial))
		s.partial = nil
	}
	return len(p), nil
}

// Printf writes a platform message (not app output) into the stream.
func (s *Stream) Printf(format string, args ...any) {
	fmt.Fprintf(s, "==> "+format+"\n", args...)
}

func (s *Stream) writeLine(line string) {
	now := time.Now()
	line = now.Format(time.RFC3339) + " " + line
	if err := s.rotate(now); err == nil {
		n, _ := s.file.WriteString(line + "\n")
		s.size += int64(n)
	}
	s.recent = append(s.recent, line)
	if len(s.recent) > tailLines {
		s.recent = s.recent[len(s.recent)-tailLines:]
	}
	for ch := range s.subs {
		select {
		case ch <- line:
		default:
		}
	}
}

func (s *Stream) rotate(now time.Time) error {
	if s.fixed != "" {
		if s.file != nil {
			return nil
		}
		return s.open(s.fixed)
	}
	day := now.Format("2006-01-02")
	if s.file != nil && day == s.day && s.size < maxFileSize {
		return nil
	}
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	if day != s.day {
		s.day, s.part = day, 0
	} else {
		s.part++
	}
	return s.open(s.fileName(day, s.part))
}

func (s *Stream) open(path string) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	s.file, s.size = f, info.Size()
	return nil
}

func (s *Stream) fileName(day string, part int) string {
	name := s.prefix + "-" + day
	if part > 0 {
		name += fmt.Sprintf(".%d", part)
	}
	return filepath.Join(s.dir, name+".log")
}

// Subscribe returns the recent lines and a channel of new ones. Call the cancel func when done.
func (s *Stream) Subscribe() ([]string, <-chan string, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan string, 256)
	s.subs[ch] = struct{}{}
	recent := append([]string(nil), s.recent...)
	return recent, ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.partial) > 0 {
		s.writeLine(string(s.partial))
		s.partial = nil
	}
	for ch := range s.subs {
		close(ch)
		delete(s.subs, ch)
	}
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

// Days lists the dates that have log files, newest first.
func Days(dir, prefix string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, prefix+"-*.log"))
	seen := map[string]bool{}
	var days []string
	for _, m := range matches {
		day := strings.TrimPrefix(filepath.Base(m), prefix+"-")
		day = day[:min(len(day), 10)]
		if !seen[day] {
			seen[day] = true
			days = append(days, day)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days
}

// ReadDay writes every log file for the given day to w, in order.
func ReadDay(w io.Writer, dir, prefix, day string) error {
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return fmt.Errorf("invalid date %q", day)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, prefix+"-"+day+"*.log"))
	sort.Slice(matches, func(i, j int) bool { return partOf(matches[i]) < partOf(matches[j]) })
	for _, m := range matches {
		f, err := os.Open(m)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func partOf(path string) int {
	var part int
	base := strings.TrimSuffix(filepath.Base(path), ".log")
	if i := strings.LastIndexByte(base, '.'); i >= 0 {
		fmt.Sscanf(base[i+1:], "%d", &part)
	}
	return part
}

// Prune deletes log files older than the retention period.
func Prune(dir, prefix string, keepDays int) {
	if keepDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	for _, day := range Days(dir, prefix) {
		if day >= cutoff {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(dir, prefix+"-"+day+"*.log"))
		for _, m := range matches {
			os.Remove(m)
		}
	}
}

// TailFile returns the last n lines of a file, used for finished deploy logs.
func TailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n*2 {
			lines = lines[len(lines)-n:]
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, sc.Err()
}
