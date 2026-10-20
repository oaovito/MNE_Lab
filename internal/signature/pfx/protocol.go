package pfx

import (
	"encoding/binary"
	"io"
)

// WorkerFlag is the only argument the worker is started with.
const WorkerFlag = "--pkcs12-worker-v1"

const (
	protocolVersion = 1
	nonceSize       = 32
	digestSize      = 32
	// maxFrame fits a load request: the file, the password and a header.
	maxFrame     = MaxBytes + MaxPasswordBytes + 64
	maxSignature = 1024 // RSA-8192
)

// Frame kinds. Each frame is a 4-byte big-endian length followed by the
// kind byte and the payload.
const (
	kindHello     byte = 'H' // parent: version, nonce
	kindReady     byte = 'R' // worker: version, nonce
	kindLoad      byte = 'L' // parent: password length (2 bytes), password, file
	kindChain     byte = 'C' // worker: certificates, leaf first, each length-prefixed
	kindSign      byte = 'S' // parent: SHA-256 digest
	kindSignature byte = 'G' // worker: signature
	kindError     byte = 'E' // worker: error code
	kindQuit      byte = 'Q' // parent: stop
)

// wipe clears a buffer that held a password or a frame carrying one.
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func writeFrame(w io.Writer, kind byte, payload ...[]byte) error {
	size := 1
	for _, p := range payload {
		size += len(p)
	}
	if size > maxFrame {
		return ErrLimit
	}
	frame := make([]byte, 0, 4+size)
	frame = binary.BigEndian.AppendUint32(frame, uint32(size))
	frame = append(frame, kind)
	for _, p := range payload {
		frame = append(frame, p...)
	}
	defer wipe(frame)
	_, err := w.Write(frame)
	return err
}

// readFrame reads one frame; the caller wipes the payload when it held a
// secret.
func readFrame(r io.Reader) (byte, []byte, error) {
	var head [5]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	size := binary.BigEndian.Uint32(head[:4])
	if size == 0 || size > maxFrame {
		return 0, nil, ErrWorker
	}
	payload := make([]byte, size-1)
	if _, err := io.ReadFull(r, payload); err != nil {
		wipe(payload)
		return 0, nil, err
	}
	return head[4], payload, nil
}
