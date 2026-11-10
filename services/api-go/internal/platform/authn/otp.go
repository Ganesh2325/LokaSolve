package authn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
)

type Message struct {
	Channel     string
	Destination string
	Body        string
	Code        string
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// LogSender records that a code was dispatched without writing the code.
type LogSender struct{}

func (LogSender) Send(context.Context, Message) error {
	return nil
}

// DevFileSender writes the code to a gitignored file only when explicitly enabled.
// API responses still do not include the code.
type DevFileSender struct {
	Path string
}

func (s DevFileSender) Send(_ context.Context, msg Message) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	line := fmt.Sprintf("channel=%s destination=%s purpose_body=%s code=%s\n", msg.Channel, msg.Destination, msg.Body, msg.Code)
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

// CapturingSender is the in-process test double.
type CapturingSender struct {
	mu   sync.Mutex
	Last map[string]string
}

func NewCapturingSender() *CapturingSender {
	return &CapturingSender{Last: map[string]string{}}
}

func (s *CapturingSender) Send(_ context.Context, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Last[msg.Destination] = msg.Code
	return nil
}

func (s *CapturingSender) Code(destination string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Last[destination]
}

type FailingSender struct{}

func (FailingSender) Send(context.Context, Message) error {
	return fmt.Errorf("otp sender unavailable")
}

func NewCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func HashCode(pepper, code string) string {
	sum := sha256.Sum256([]byte(pepper + ":" + code))
	return hex.EncodeToString(sum[:])
}

func CodeMatches(pepper, code, encoded string) bool {
	got := HashCode(pepper, code)
	if len(got) != len(encoded) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(encoded)) == 1
}
