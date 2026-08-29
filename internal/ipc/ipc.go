//go:build windows

package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// Message is the handover payload sent from a secondary process to the owner.
type Message struct {
	// Paths are the files/folders the secondary process was asked to convert.
	Paths []string `json:"paths"`
	// Preset optionally overrides the entry point's bound preset (the --preset
	// flag). Empty means use the binding.
	Preset string `json:"preset,omitempty"`
}

const (
	maxMessageBytes = 8 << 20 // 8 MiB guard against a runaway length prefix
	ackByte         = 0x06    // ASCII ACK
)

// Acquire implements single-instance startup. It first tries to become the
// owner by creating the pipe; if an owner already exists it hands msg over and
// reports handedOver=true (the caller should exit). If the existing owner is
// unreachable (crashed or hung) it takes over by becoming the owner itself.
func Acquire(msg Message, timeout time.Duration) (server *Server, handedOver bool, err error) {
	name, err := PipeName()
	if err != nil {
		return nil, false, err
	}
	return AcquireName(name, msg, timeout)
}

// AcquireName is Acquire against an explicit pipe name (used by tests).
func AcquireName(name string, msg Message, timeout time.Duration) (*Server, bool, error) {
	// Try to own the pipe. ListenPipe uses FILE_FLAG_FIRST_PIPE_INSTANCE, so
	// this succeeds for exactly one process; a second owner cannot appear.
	if srv, err := ListenName(name); err == nil {
		return srv, false, nil
	}

	// An owner appears to exist: hand over.
	sent, sendErr := SendName(name, msg, timeout)
	if sent {
		return nil, true, nil
	}

	// The owner is unreachable (it died between our attempts, or is hung).
	// Take over by becoming the owner.
	if srv, err := ListenName(name); err == nil {
		return srv, false, nil
	}
	if sendErr != nil {
		return nil, false, sendErr
	}
	return nil, false, errors.New("ipc: could not acquire ownership or reach an owner")
}

// TrySend delivers msg to the current user's owner. See SendName for semantics.
func TrySend(msg Message, timeout time.Duration) (bool, error) {
	name, err := PipeName()
	if err != nil {
		return false, err
	}
	return SendName(name, msg, timeout)
}

// SendName connects to an owner on the given pipe and delivers msg. It returns
// (true, nil) when acknowledged, (false, nil) when no owner is listening, and
// (false, err) on an unexpected error.
func SendName(name string, msg Message, timeout time.Duration) (bool, error) {
	conn, err := winio.DialPipe(name, &timeout)
	if err != nil {
		if errors.Is(err, winio.ErrTimeout) || isFileNotFound(err) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err := writeMessage(conn, msg); err != nil {
		return false, err
	}
	var ack [1]byte
	if _, err := io.ReadFull(conn, ack[:]); err != nil {
		return false, err
	}
	if ack[0] != ackByte {
		return false, errors.New("ipc: owner did not acknowledge handover")
	}
	return true, nil
}

// Server is the owning process's pipe listener.
type Server struct {
	ln   net.Listener
	name string
}

// Listen becomes the owner on the current user's pipe.
func Listen() (*Server, error) {
	name, err := PipeName()
	if err != nil {
		return nil, err
	}
	return ListenName(name)
}

// ListenName becomes the owner on an explicit pipe name (used by tests).
func ListenName(name string) (*Server, error) {
	ln, err := winio.ListenPipe(name, &winio.PipeConfig{MessageMode: false})
	if err != nil {
		return nil, err
	}
	return &Server{ln: ln, name: name}, nil
}

// Serve accepts handovers until the server is closed, invoking handler for each
// received message. Handovers are tiny, so connections are handled one at a time
// to keep ordering simple; each sender still gets its ack and exits immediately.
func (s *Server) Serve(handler func(Message)) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // listener closed
		}
		s.handleConn(conn, handler)
	}
}

func (s *Server) handleConn(conn net.Conn, handler func(Message)) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	msg, err := readMessage(conn)
	if err != nil {
		return
	}
	// Acknowledge before running the handler so the sender can exit at once.
	_, _ = conn.Write([]byte{ackByte})
	if handler != nil {
		handler(msg)
	}
}

// Close stops accepting new handovers.
func (s *Server) Close() error {
	return s.ln.Close()
}

func writeMessage(w io.Writer, msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func readMessage(r io.Reader) (Message, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Message{}, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > maxMessageBytes {
		return Message{}, errors.New("ipc: invalid message length")
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Message{}, err
	}
	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

// isFileNotFound reports whether err indicates the pipe does not exist (no
// owner). winio.DialPipe surfaces ERROR_FILE_NOT_FOUND in that case.
func isFileNotFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND)
}
