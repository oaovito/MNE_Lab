package restartpipe

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func frame(body []byte, size uint32) []byte {
	b := make([]byte, 4, 4+len(body))
	binary.BigEndian.PutUint32(b, size)
	return append(b, body...)
}

func encodedMessage(t *testing.T, m Message) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return frame(b, uint32(len(b)))
}

func TestResumeAndStatusRoundTrips(t *testing.T) {
	payload := []byte(`{"account":"synthetic-account","ak":"test-only-key-marker","profile":"synthetic-profile","pdk":"test-only-profile-key"}`)
	original := bytes.Clone(payload)
	var stream bytes.Buffer
	w := New(nil, &stream)
	if err := w.Write("prepared", 7, nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("resume", 0, payload); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"ready", "commit", "committed"} {
		if err := w.Write(phase, 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(payload, original) {
		t.Fatal("writing a frame changed the caller's payload")
	}
	r := New(&stream, nil)
	for _, phase := range []string{"prepared", "resume", "ready", "commit", "committed"} {
		m, err := r.Read(phase)
		if err != nil {
			t.Fatalf("read %s: %v", phase, err)
		}
		if m.Protocol != Protocol || m.Phase != phase {
			t.Fatalf("incorrect envelope for %s: %+v", phase, m)
		}
		if phase == "prepared" && m.Schema != 7 {
			t.Fatalf("prepared schema=%d, want 7", m.Schema)
		}
		if phase == "resume" {
			if !bytes.Equal(m.Payload, original) {
				t.Fatal("resume payload was not preserved after the frame buffer was wiped")
			}
		} else if len(m.Payload) != 0 {
			t.Fatalf("status %s contains a payload", phase)
		}
	}
	if stream.Len() != 0 {
		t.Fatalf("round trips left %d unread bytes", stream.Len())
	}
}

func TestReadRejectsTruncatedFrames(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{"empty header", nil, io.EOF},
		{"one header byte", []byte{0}, io.ErrUnexpectedEOF},
		{"three header bytes", []byte{0, 0, 0}, io.ErrUnexpectedEOF},
		{"empty body", frame(nil, 5), io.EOF},
		{"partial body", frame([]byte("ab"), 5), io.ErrUnexpectedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := New(bytes.NewReader(tc.data), nil).Read("ready")
			if !errors.Is(err, tc.want) || m.Protocol != "" || len(m.Payload) != 0 {
				t.Fatalf("truncated frame returned message=%+v error=%v, want %v", m, err, tc.want)
			}
		})
	}
}

func TestReadRejectsInvalidSizeWithoutConsumingBody(t *testing.T) {
	for _, size := range []uint32{0, MaxFrame + 1, ^uint32(0)} {
		body := []byte("unread synthetic payload")
		input := bytes.NewBuffer(frame(body, size))
		m, err := New(input, nil).Read("resume")
		if !errors.Is(err, ErrProtocol) || len(m.Payload) != 0 {
			t.Fatalf("size %d returned message=%+v error=%v", size, m, err)
		}
		if !bytes.Equal(input.Bytes(), body) {
			t.Fatalf("size %d consumed payload bytes before rejecting the frame", size)
		}
	}
}

func TestReadRejectsInvalidEnvelopeWithoutEchoingPayload(t *testing.T) {
	const marker = "private-test-payload-must-not-reach-errors"
	valid := Message{Protocol: Protocol, Phase: "ready"}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"invalid JSON", frame([]byte(`{"secret":"`+marker), uint32(len(`{"secret":"`+marker)))},
		{"null envelope", frame([]byte("null"), 4)},
		{"wrong version", encodedMessage(t, Message{Protocol: "mnelab.restart/unknown", Phase: "ready"})},
		{"wrong phase", encodedMessage(t, Message{Protocol: Protocol, Phase: "resume"})},
		{"missing version", encodedMessage(t, Message{Phase: "ready"})},
		{"missing phase", encodedMessage(t, Message{Protocol: Protocol})},
		{"unexpected next message", encodedMessage(t, Message{Protocol: valid.Protocol, Phase: "committed"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := New(bytes.NewReader(tc.data), nil).Read("ready")
			if !errors.Is(err, ErrProtocol) || m.Protocol != "" || len(m.Payload) != 0 {
				t.Fatalf("invalid envelope returned message=%+v error=%v", m, err)
			}
			if strings.Contains(err.Error(), marker) {
				t.Fatal("protocol error exposed the rejected payload")
			}
		})
	}
}

func TestStatusMessagesForbidPayloads(t *testing.T) {
	const marker = "synthetic-private-payload"
	for _, phase := range []string{"prepared", "ready", "commit", "committed"} {
		for _, payload := range []json.RawMessage{json.RawMessage(`{"key":"` + marker + `"}`), json.RawMessage("null")} {
			t.Run(phase+"/"+string(payload), func(t *testing.T) {
				input := encodedMessage(t, Message{Protocol: Protocol, Phase: phase, Payload: payload})
				m, err := New(bytes.NewReader(input), nil).Read(phase)
				if !errors.Is(err, ErrProtocol) || len(m.Payload) != 0 {
					t.Fatalf("status payload accepted: message=%+v error=%v", m, err)
				}
				if strings.Contains(err.Error(), marker) || strings.Contains(err.Error(), string(payload)) {
					t.Fatal("protocol error exposed a forbidden status payload")
				}
			})
		}
	}
}

func TestWriteRejectsInvalidAndOversizedPayloads(t *testing.T) {
	const marker = "private-test-payload-must-not-reach-errors"
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{"invalid JSON", []byte(`{"key":"` + marker)},
		{"oversized JSON", []byte(`{"key":"` + marker + strings.Repeat("x", MaxFrame) + `"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			err := New(nil, &output).Write("resume", 0, tc.payload)
			if !errors.Is(err, ErrProtocol) {
				t.Fatalf("invalid payload returned %v", err)
			}
			if output.Len() != 0 {
				t.Fatal("rejected frame wrote bytes to the pipe")
			}
			if strings.Contains(err.Error(), marker) {
				t.Fatal("write error exposed the rejected payload")
			}
		})
	}
}

func TestMaximumFrameBoundary(t *testing.T) {
	base := Message{Protocol: Protocol, Phase: "resume", Payload: json.RawMessage(`{"padding":""}`)}
	body, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"padding":"` + strings.Repeat("x", MaxFrame-len(body)) + `"}`)
	var output bytes.Buffer
	if err := New(nil, &output).Write("resume", 0, payload); err != nil {
		t.Fatalf("maximum-size valid frame rejected: %v", err)
	}
	if got := binary.BigEndian.Uint32(output.Bytes()[:4]); got != MaxFrame {
		t.Fatalf("boundary fixture has body size %d, want %d", got, MaxFrame)
	}
	m, err := New(&output, nil).Read("resume")
	if err != nil || !bytes.Equal(m.Payload, payload) {
		t.Fatalf("maximum-size frame did not round trip: %v", err)
	}
}

type writeProbe struct {
	call   int
	failAt int
	short  bool
	err    error
}

func (w *writeProbe) Write(b []byte) (int, error) {
	w.call++
	if w.call == w.failAt {
		if w.short {
			return len(b) - 1, nil
		}
		return 0, w.err
	}
	return len(b), nil
}

func TestWritePropagatesShortWritesAndIOErrors(t *testing.T) {
	sentinel := errors.New("synthetic writer failure")
	for _, failAt := range []int{1, 2} {
		for _, short := range []bool{false, true} {
			w := &writeProbe{failAt: failAt, short: short, err: sentinel}
			want := error(sentinel)
			if short {
				want = io.ErrShortWrite
			}
			err := New(nil, w).Write("ready", 0, nil)
			if !errors.Is(err, want) {
				t.Fatalf("write call %d, short=%v: got %v, want %v", failAt, short, err, want)
			}
			if w.call != failAt {
				t.Fatalf("writer continued after failure: calls=%d, failed call=%d", w.call, failAt)
			}
		}
	}
}

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }

func TestReadPropagatesIOErrors(t *testing.T) {
	sentinel := errors.New("synthetic reader failure")
	for _, reader := range []io.Reader{
		failedReader{sentinel},
		io.MultiReader(bytes.NewReader(frame(nil, 5)), failedReader{sentinel}),
	} {
		m, err := New(reader, nil).Read("ready")
		if !errors.Is(err, sentinel) || m.Protocol != "" || len(m.Payload) != 0 {
			t.Fatalf("I/O failure returned message=%+v error=%v", m, err)
		}
	}
}
