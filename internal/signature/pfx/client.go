package pfx

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// Parent deadlines. A worker that misses one is killed.
const (
	LoadTimeout   = 20 * time.Second
	SignTimeout   = 10 * time.Second
	closeTimeout  = 2 * time.Second
	maxWorkers    = 2
	maxChainCerts = 32
)

var workers = make(chan struct{}, maxWorkers)

// Options select the worker executable and deadlines.
type Options struct {
	// Executable is the application's own executable, started with
	// WorkerFlag and nothing else.
	Executable  string
	LoadTimeout time.Duration
	SignTimeout time.Duration

	args []string // tests start other programs
	dir  string
}

// Credential is an opened credential. Its private key stays in the worker;
// Sign sends only SHA-256 digests there and checks every signature against
// the certificate before returning it. Close ends the worker.
type Credential struct {
	certificate *x509.Certificate
	chain       []*x509.Certificate
	signTimeout time.Duration

	mu     sync.Mutex
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    io.ReadCloser
	done   chan struct{}
	closed bool
	// The worker's slot and job are returned once, when it exits however
	// it ends, so a credential that is never closed holds no slot.
	release func()
	freed   sync.Once
}

func (c *Credential) free() {
	c.freed.Do(func() {
		c.release()
		<-workers
	})
}

// Open checks a credential file without its password, then starts a worker
// to decode it. The password is sent only over the worker's pipe; the
// frame that carried it is wiped, and the caller keeps and wipes its own
// slice. Cancelling ctx before Open returns kills the worker and returns
// ctx's error; a worker that misses the load deadline returns ErrTimeout.
func Open(ctx context.Context, opt Options, file, password []byte) (*Credential, error) {
	if len(password) > MaxPasswordBytes {
		return nil, ErrLimit
	}
	cost, err := Preflight(file)
	if err == nil && len(password) == 0 {
		err = cost.EmptyPassword()
	}
	if err != nil {
		return nil, err
	}
	if opt.Executable == "" {
		return nil, ErrWorker
	}
	loadTimeout, signTimeout := opt.LoadTimeout, opt.SignTimeout
	if loadTimeout <= 0 {
		loadTimeout = LoadTimeout
	}
	if signTimeout <= 0 {
		signTimeout = SignTimeout
	}
	select {
	case workers <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c, err := start(opt)
	if err != nil {
		return nil, err
	}
	c.signTimeout = signTimeout
	loading, cancel := context.WithTimeout(ctx, loadTimeout)
	defer cancel()
	stop := context.AfterFunc(loading, c.kill)
	err = c.load(file, password)
	if !stop() || loading.Err() != nil {
		// The caller's cancellation is reported as such; the load
		// deadline as a timeout.
		err = ctx.Err()
		if err == nil {
			err = ErrTimeout
		}
	}
	if err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// start launches a worker in a slot the caller holds; the slot is
// returned when the worker exits or fails to start.
func start(opt Options) (*Credential, error) {
	args := opt.args
	if args == nil {
		args = []string{WorkerFlag}
	}
	cmd := exec.Command(opt.Executable, args...)
	// Nothing is inherited: no environment, standard error or working
	// folder the worker could write to.
	cmd.Env = workerEnv()
	cmd.Dir = opt.dir
	if cmd.Dir == "" {
		cmd.Dir = os.TempDir()
	}
	prepare(cmd)
	// The pipes are created here rather than by exec, whose Wait would
	// close the parent's ends as soon as the worker exits, before its last
	// frame is read.
	childIn, in, err := os.Pipe()
	if err != nil {
		<-workers
		return nil, ErrWorker
	}
	out, childOut, err := os.Pipe()
	if err != nil {
		childIn.Close()
		in.Close()
		<-workers
		return nil, ErrWorker
	}
	cmd.Stdin, cmd.Stdout = childIn, childOut
	err = cmd.Start()
	childIn.Close()
	childOut.Close()
	if err != nil {
		in.Close()
		out.Close()
		<-workers
		return nil, ErrWorker
	}
	// The job is attached while exec still holds the process, before Wait
	// can release its identifier.
	release, err := contain(cmd)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		in.Close()
		out.Close()
		<-workers
		return nil, ErrWorker
	}
	c := &Credential{cmd: cmd, in: in, out: out, done: make(chan struct{}), release: release}
	go func() {
		cmd.Wait()
		close(c.done)
		c.free()
	}()
	return c, nil
}

// workerEnv keeps only what the operating system needs to start a process.
func workerEnv() []string {
	if runtime.GOOS != "windows" {
		return []string{}
	}
	var env []string
	for _, name := range []string{"SystemRoot", "SYSTEMROOT"} {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
			break
		}
	}
	return env
}

// load runs the handshake, sends the credential and proves that the worker
// holds the key of the certificate it reports.
func (c *Credential) load(file, password []byte) error {
	nonce := make([]byte, 1+nonceSize)
	nonce[0] = protocolVersion
	if _, err := rand.Read(nonce[1:]); err != nil {
		return ErrWorker
	}
	if writeFrame(c.in, kindHello, nonce) != nil {
		return ErrWorker
	}
	kind, ready, err := readFrame(c.out)
	if err != nil || kind != kindReady || subtle.ConstantTimeCompare(ready, nonce) != 1 {
		return ErrWorker
	}
	size := binary.BigEndian.AppendUint16(nil, uint16(len(password)))
	if writeFrame(c.in, kindLoad, size, password, file) != nil {
		return ErrWorker
	}
	kind, payload, err := readFrame(c.out)
	if err != nil {
		return ErrWorker
	}
	if kind == kindError {
		return workerError(string(payload))
	}
	if kind != kindChain {
		return ErrWorker
	}
	chain, err := parseChain(payload)
	if err != nil {
		return err
	}
	c.certificate, c.chain = chain[0], chain[1:]
	challenge := sha256.Sum256(append([]byte("mnelab.pfx.possession:"), nonce...))
	_, err = c.sign(challenge[:])
	return err
}

func parseChain(payload []byte) ([]*x509.Certificate, error) {
	var chain []*x509.Certificate
	for len(payload) > 0 {
		if len(payload) < 4 || len(chain) >= maxChainCerts {
			return nil, ErrWorker
		}
		n := int(binary.BigEndian.Uint32(payload))
		if n > maxCertificate || n > len(payload)-4 {
			return nil, ErrWorker
		}
		cert, err := x509.ParseCertificate(payload[4 : 4+n])
		if err != nil {
			return nil, ErrWorker
		}
		chain = append(chain, cert)
		payload = payload[4+n:]
	}
	if len(chain) == 0 || !usable(chain[0].PublicKey) {
		return nil, ErrCredential
	}
	return chain, nil
}

func workerError(code string) error {
	for _, e := range []error{ErrMalformed, ErrUnsupported, ErrLimit, ErrWrongPassword, ErrCredential} {
		if "pfx."+code == e.Error() {
			return e
		}
	}
	return ErrWorker
}

// Certificate returns the signing certificate.
func (c *Credential) Certificate() *x509.Certificate { return c.certificate }

// Chain returns the other certificates in the file, as the file lists them.
func (c *Credential) Chain() []*x509.Certificate { return c.chain }

// Public returns the certificate's public key.
func (c *Credential) Public() crypto.PublicKey { return c.certificate.PublicKey }

// Sign signs a SHA-256 digest with RSA PKCS #1 v1.5 or ECDSA. Other hashes,
// RSA-PSS and digests of another length are refused. A worker that misses
// the deadline, breaks the protocol or returns a signature that does not
// verify is ended, and the credential is closed.
func (c *Credential) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if _, pss := opts.(*rsa.PSSOptions); pss || opts == nil || opts.HashFunc() != crypto.SHA256 || len(digest) != digestSize {
		return nil, ErrUnsupported
	}
	return c.sign(digest)
}

func (c *Credential) sign(digest []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, ErrClosed
	}
	timer := time.AfterFunc(c.signTimeout, c.kill)
	defer timer.Stop()
	var kind byte
	var sig []byte
	err := writeFrame(c.in, kindSign, digest)
	if err == nil {
		kind, sig, err = readFrame(c.out)
	}
	if !timer.Stop() {
		return nil, c.failLocked(ErrTimeout)
	}
	if err != nil || kind != kindSignature || len(sig) == 0 || len(sig) > maxSignature {
		return nil, c.failLocked(ErrWorker)
	}
	if !verify(c.certificate.PublicKey, digest, sig) {
		return nil, c.failLocked(ErrCredential)
	}
	return sig, nil
}

// failLocked ends a worker that broke the protocol or a deadline, or whose
// signature does not match its certificate.
func (c *Credential) failLocked(err error) error {
	c.closeLocked()
	return err
}

func verify(key crypto.PublicKey, digest, sig []byte) bool {
	switch k := key.(type) {
	case *rsa.PublicKey:
		return rsa.VerifyPKCS1v15(k, crypto.SHA256, digest, sig) == nil
	case *ecdsa.PublicKey:
		return ecdsa.VerifyASN1(k, digest, sig)
	}
	return false
}

func (c *Credential) kill() {
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
}

// Close ends the worker, which takes the key with it. It is safe to call
// more than once.
func (c *Credential) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
	return nil
}

func (c *Credential) closeLocked() {
	if c.closed {
		return
	}
	c.closed = true
	writeFrame(c.in, kindQuit)
	c.in.Close()
	select {
	case <-c.done:
	case <-time.After(closeTimeout):
		c.kill()
		<-c.done
	}
	c.out.Close()
	c.free()
}
