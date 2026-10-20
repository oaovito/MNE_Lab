package pfx

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

// Worker deadlines. Each one ends the process; the parent applies its own,
// shorter ones and kills the worker as well.
const (
	workerHelloTimeout = 5 * time.Second
	workerLoadTimeout  = 30 * time.Second
	workerIdleTimeout  = 5 * time.Minute
	workerLifetime     = 15 * time.Minute
	workerMemoryLimit  = 256 << 20
	maxSignatures      = 4096
)

// terminate ends the worker when a deadline passes, even inside a key
// derivation, which cannot be interrupted otherwise.
var terminate = func() { os.Exit(3) }

// Serve runs the worker on the pipes the parent started it with and returns
// the exit status. It reads nothing else: no arguments beyond WorkerFlag,
// environment, files or settings, and it writes only to out.
func Serve(in io.Reader, out io.Writer) int {
	limitProcess()
	debug.SetMemoryLimit(workerMemoryLimit)
	life := time.AfterFunc(workerLifetime, terminate)
	defer life.Stop()
	deadline := time.AfterFunc(workerHelloTimeout, terminate)
	defer deadline.Stop()

	kind, hello, err := readFrame(in)
	if err != nil || kind != kindHello || len(hello) != 1+nonceSize || hello[0] != protocolVersion {
		return 2
	}
	if writeFrame(out, kindReady, hello) != nil {
		return 2
	}
	deadline.Reset(workerLoadTimeout)
	kind, load, err := readFrame(in)
	if err != nil || kind != kindLoad {
		wipe(load)
		return 2
	}
	key, chain, code := open(load)
	wipe(load)
	if code != "" {
		writeFrame(out, kindError, []byte(code))
		return 1
	}
	var certs [][]byte
	for _, c := range chain {
		certs = append(certs, binary.BigEndian.AppendUint32(nil, uint32(len(c.Raw))), c.Raw)
	}
	if writeFrame(out, kindChain, certs...) != nil {
		return 2
	}
	for signed := 0; ; signed++ {
		deadline.Reset(workerIdleTimeout)
		kind, digest, err := readFrame(in)
		switch {
		case err == io.EOF || err == nil && kind == kindQuit:
			return 0
		case err != nil || kind != kindSign || len(digest) != digestSize || signed >= maxSignatures:
			return 2
		}
		sig, err := sign(key, digest)
		if err != nil {
			writeFrame(out, kindError, []byte("credential"))
			return 1
		}
		if writeFrame(out, kindSignature, sig) != nil {
			return 2
		}
	}
}

// open decodes the credential in a load request. It returns an error code
// for the parent instead of an error, so no decoder message, which can
// describe the file, leaves the worker.
func open(load []byte) (crypto.Signer, []*x509.Certificate, string) {
	if len(load) < 2 {
		return nil, nil, "protocol"
	}
	n := int(binary.BigEndian.Uint16(load))
	if n > MaxPasswordBytes || len(load) < 2+n {
		return nil, nil, "protocol"
	}
	password, file := load[2:2+n], load[2+n:]
	cost, err := Preflight(file)
	if err == nil && n == 0 {
		err = cost.EmptyPassword()
	}
	if err != nil {
		return nil, nil, code(err)
	}
	// The decoder takes the password as a string, a copy that cannot be
	// wiped; it lasts no longer than this process.
	key, leaf, cas, err := pkcs12.DecodeChain(file, string(password))
	switch {
	case errors.Is(err, pkcs12.ErrIncorrectPassword) || errors.Is(err, pkcs12.ErrDecryption):
		return nil, nil, "wrong_password"
	case errors.As(err, new(pkcs12.NotImplementedError)):
		return nil, nil, "unsupported"
	case err != nil:
		return nil, nil, "malformed"
	}
	signer, ok := key.(crypto.Signer)
	if !ok || !usable(signer.Public()) {
		return nil, nil, "credential"
	}
	// The decoder takes the first certificate as the leaf; files list the
	// chain in any order, so the leaf is the one that matches the key.
	chain := append([]*x509.Certificate{leaf}, cas...)
	for i, c := range chain {
		if matches(signer.Public(), c.PublicKey) {
			chain[0], chain[i] = chain[i], chain[0]
			return signer, chain, ""
		}
	}
	return nil, nil, "credential"
}

func usable(key crypto.PublicKey) bool {
	switch k := key.(type) {
	case *rsa.PublicKey:
		return k.N.BitLen() >= 2048 && k.N.BitLen() <= 8192
	case *ecdsa.PublicKey:
		return k.Curve == elliptic.P256() || k.Curve == elliptic.P384() || k.Curve == elliptic.P521()
	}
	return false
}

func matches(key, certificate crypto.PublicKey) bool {
	k, ok := key.(interface{ Equal(crypto.PublicKey) bool })
	return ok && k.Equal(certificate)
}

func sign(key crypto.Signer, digest []byte) ([]byte, error) {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, digest)
	case *ecdsa.PrivateKey:
		return ecdsa.SignASN1(rand.Reader, k, digest)
	}
	return nil, ErrCredential
}

func code(err error) string {
	for _, e := range []error{ErrMalformed, ErrUnsupported, ErrLimit, ErrCredential} {
		if errors.Is(err, e) {
			return strings.TrimPrefix(e.Error(), "pfx.")
		}
	}
	return "malformed"
}
