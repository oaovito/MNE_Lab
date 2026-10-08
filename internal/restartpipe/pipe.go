// Package restartpipe exchanges bounded restart messages over inherited
// stdin/stdout pipes. Session keys appear only in the resume message.
package restartpipe

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

const Protocol = "mnelab.restart/1"
const MaxFrame = 8 << 10

var ErrProtocol = errors.New("restart.handoff_invalid")

type Message struct {
	Protocol string          `json:"protocol"`
	Phase    string          `json:"phase"`
	Schema   int             `json:"schema,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type Pipe struct {
	in  io.Reader
	out io.Writer
}

func New(in io.Reader, out io.Writer) *Pipe { return &Pipe{in: in, out: out} }

func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func (p *Pipe) Write(phase string, schema int, payload []byte) error {
	b, err := json.Marshal(Message{Protocol: Protocol, Phase: phase, Schema: schema, Payload: payload})
	if err != nil || len(b) > MaxFrame {
		Wipe(b)
		return ErrProtocol
	}
	defer Wipe(b)
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(b)))
	if err := writeFull(p.out, size[:]); err != nil {
		return err
	}
	return writeFull(p.out, b)
}

func writeFull(w io.Writer, b []byte) error {
	n, err := w.Write(b)
	if err == nil && n != len(b) {
		return io.ErrShortWrite
	}
	return err
}

func (p *Pipe) Read(phase string) (Message, error) {
	var size [4]byte
	if _, err := io.ReadFull(p.in, size[:]); err != nil {
		return Message{}, err
	}
	n := binary.BigEndian.Uint32(size[:])
	if n == 0 || n > MaxFrame {
		return Message{}, ErrProtocol
	}
	b := make([]byte, n)
	defer Wipe(b)
	if _, err := io.ReadFull(p.in, b); err != nil {
		return Message{}, err
	}
	var m Message
	if err := json.Unmarshal(b, &m); err != nil || m.Protocol != Protocol || m.Phase != phase || (phase != "resume" && len(m.Payload) != 0) {
		Wipe(m.Payload)
		return Message{}, ErrProtocol
	}
	return m, nil
}
