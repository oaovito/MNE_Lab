package pfx

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/signature"
	"software.sslmate.com/src/go-pkcs12"
)

// fakeFlag starts the test binary as a worker that breaks the protocol in
// the way its argument names.
const fakeFlag = "--pfx-test-fake"

const testPassword = "invented-test-password-7Q"

func TestMain(m *testing.M) {
	switch {
	case len(os.Args) == 2 && os.Args[1] == WorkerFlag:
		os.Exit(Serve(os.Stdin, os.Stdout))
	case len(os.Args) == 3 && os.Args[1] == fakeFlag:
		os.Exit(fakeServe(os.Args[2]))
	}
	os.Exit(m.Run())
}

// fakeServe answers like a worker, then misbehaves.
func fakeServe(mode string) int {
	in, out := os.Stdin, os.Stdout
	_, hello, err := readFrame(in)
	if err != nil {
		return 2
	}
	switch mode {
	case "hang":
		time.Sleep(time.Hour)
	case "nonce":
		hello[len(hello)-1] ^= 1
	}
	writeFrame(out, kindReady, hello)
	_, load, err := readFrame(in)
	if err != nil {
		return 2
	}
	if mode == "slow" {
		// A decoder busy far past every deadline.
		time.Sleep(time.Hour)
	}
	key, chain, code := open(load)
	if code != "" {
		writeFrame(out, kindError, []byte(code))
		return 1
	}
	if mode == "certificate" {
		chain = []*x509.Certificate{testCertificate(nil, mustEC())}
	}
	var certs [][]byte
	for _, c := range chain {
		certs = append(certs, binary.BigEndian.AppendUint32(nil, uint32(len(c.Raw))), c.Raw)
	}
	writeFrame(out, kindChain, certs...)
	for n := 0; ; n++ {
		_, digest, err := readFrame(in)
		if err != nil {
			return 0
		}
		sig, _ := sign(key, digest)
		switch {
		case mode == "forge":
			sig = make([]byte, len(sig))
			rand.Read(sig)
		case mode == "stall" && n > 0:
			time.Sleep(time.Hour)
		}
		writeFrame(out, kindSignature, sig)
		if mode == "exit" {
			return 0
		}
	}
}

func mustEC() *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return key
}

func testCertificate(t *testing.T, key crypto.Signer) *x509.Certificate {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "Invented PFX test signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err == nil {
		var cert *x509.Certificate
		if cert, err = x509.ParseCertificate(raw); err == nil {
			return cert
		}
	}
	if t != nil {
		t.Fatal(err)
	}
	panic(err)
}

var fixtures = sync.OnceValues(func() (map[string]crypto.Signer, error) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return map[string]crypto.Signer{"RSA": rsaKey, "ECDSA": mustEC()}, nil
})

func testKey(t *testing.T, name string) crypto.Signer {
	t.Helper()
	keys, err := fixtures()
	if err != nil {
		t.Fatal(err)
	}
	return keys[name]
}

func encode(t *testing.T, enc *pkcs12.Encoder, key crypto.Signer, cert *x509.Certificate, cas []*x509.Certificate) []byte {
	t.Helper()
	raw, err := enc.Encode(key, cert, cas, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func options(t *testing.T) Options {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Options{Executable: exe, dir: t.TempDir()}
}

func fake(t *testing.T, mode string) Options {
	opt := options(t)
	opt.args = []string{fakeFlag, mode}
	return opt
}

// idle reports whether every worker slot is free again.
func idle(t *testing.T) {
	t.Helper()
	if n := len(workers); n != 0 {
		t.Fatalf("%d worker slots still held", n)
	}
}

func TestGoEncodersOpenAndSign(t *testing.T) {
	encoders := map[string]*pkcs12.Encoder{
		"Modern2023": pkcs12.Modern2023, "Modern2026": pkcs12.Modern2026,
		"LegacyDES": pkcs12.LegacyDES, "LegacyRC2": pkcs12.LegacyRC2,
	}
	for _, kind := range []string{"RSA", "ECDSA"} {
		for name, enc := range encoders {
			t.Run(kind+"/"+name, func(t *testing.T) {
				key := testKey(t, kind)
				cert := testCertificate(t, key)
				other := testCertificate(t, mustEC())
				// The other certificate comes first; the leaf is the one
				// that matches the key.
				file := encode(t, enc, key, other, []*x509.Certificate{cert})
				if _, err := Preflight(file); err != nil {
					t.Fatal(err)
				}
				signWith(t, options(t), file, cert, 1)
			})
		}
	}
}

// signWith opens a file, signs content through the CMS signer and checks
// the result.
func signWith(t *testing.T, opt Options, file []byte, want *x509.Certificate, chain int) {
	t.Helper()
	password := []byte(testPassword)
	cred, err := Open(context.Background(), opt, file, password)
	if err != nil {
		t.Fatal(err)
	}
	defer cred.Close()
	if !cred.Certificate().Equal(want) || len(cred.Chain()) != chain {
		t.Fatalf("certificate %v, chain %d", cred.Certificate().Subject, len(cred.Chain()))
	}
	content := []byte("invented,data\r\n0,1.2300\r\n\x00\xff")
	der, err := signature.SignExport(content, cred.Certificate(), cred, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := signature.VerifySignature(content, der, signature.VerifyOptions{})
	if err != nil || result.Signature != "valid" || result.Content != "matches" {
		t.Fatalf("%+v %v", result, err)
	}
	if err := cred.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := cred.Sign(nil, make([]byte, 32), crypto.SHA256); !errors.Is(err, ErrClosed) {
		t.Fatal("signed after Close:", err)
	}
	idle(t)
	entries, err := os.ReadDir(opt.dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("worker folder not empty:", entries, err)
	}
}

func TestOpenSSLFilesOpenAndSign(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("independent OpenSSL oracle unavailable")
	}
	for _, kind := range []string{"RSA", "ECDSA"} {
		key := testKey(t, kind)
		cert := testCertificate(t, key)
		dir := t.TempDir()
		private, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		keyPath := filepath.Join(dir, "key.pem")
		certPath := filepath.Join(dir, "cert.pem")
		os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0o600)
		os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600)
		for name, extra := range map[string][]string{
			"default":   nil,
			"legacy":    {"-legacy"},
			"sha512":    {"-macalg", "sha512", "-iter", "100000"},
			"plain-key": {"-keypbe", "NONE", "-certpbe", "NONE"},
		} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				out := filepath.Join(dir, name+".p12")
				args := append([]string{"pkcs12", "-export", "-inkey", keyPath, "-in", certPath, "-passout", "pass:" + testPassword, "-out", out}, extra...)
				if raw, err := exec.Command(openssl, args...).CombinedOutput(); err != nil {
					if name == "legacy" {
						t.Skipf("OpenSSL legacy provider unavailable: %s", raw)
					}
					t.Fatalf("%v: %s", err, raw)
				}
				file, err := os.ReadFile(out)
				if err != nil {
					t.Fatal(err)
				}
				signWith(t, options(t), file, cert, 0)
			})
		}
		t.Run(kind+"/nomac", func(t *testing.T) {
			out := filepath.Join(dir, "nomac.p12")
			if raw, err := exec.Command(openssl, "pkcs12", "-export", "-nomac", "-inkey", keyPath, "-in", certPath, "-passout", "pass:"+testPassword, "-out", out).CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, raw)
			}
			file, _ := os.ReadFile(out)
			if _, err := Open(context.Background(), options(t), file, []byte(testPassword)); !errors.Is(err, ErrUnsupported) {
				t.Fatal("a file without a MAC opened:", err)
			}
		})
	}
}

func TestRejectedFiles(t *testing.T) {
	key := testKey(t, "ECDSA")
	cert := testCertificate(t, key)
	good := encode(t, pkcs12.Modern2023, key, cert, nil)
	// Indefinite length: the outer SEQUENCE re-encoded as BER.
	header := 2
	if good[1]&0x80 != 0 {
		header += int(good[1] & 0x7f)
	}
	ber := append(append([]byte{0x30, 0x80}, good[header:]...), 0, 0)
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		file []byte
		want error
	}{
		"empty":          {nil, ErrMalformed},
		"truncated":      {good[:len(good)-7], ErrMalformed},
		"trailing":       {append(append([]byte{}, good...), 0), ErrMalformed},
		"BER":            {ber, ErrMalformed},
		"oversized":      {make([]byte, MaxBytes+1), ErrLimit},
		"random":         {bytes.Repeat([]byte{0x30, 0x03, 0x02, 0x01, 0x03}, 3), ErrMalformed},
		"no MAC":         {encodeNoMAC(t, key, cert), ErrUnsupported},
		"one derivation": {encode(t, pkcs12.Modern2023.WithIterations(MaxIterations+1), key, cert, nil), ErrLimit},
		// MAC once, then key and certificates with PBES1, which derives the
		// key and the IV separately: 1M + 2M + 2M.
		"total cost":   {encode(t, pkcs12.LegacyDES.WithIterations(MaxIterations), key, cert, nil), ErrLimit},
		"weak RSA key": {encode(t, pkcs12.Modern2023, weak, testCertificate(t, weak), nil), ErrCredential},
		"wrong leaf":   {encode(t, pkcs12.Modern2023, key, testCertificate(t, mustEC()), nil), ErrCredential},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Open(context.Background(), options(t), c.file, []byte(testPassword))
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			idle(t)
		})
	}
	t.Run("wrong password", func(t *testing.T) {
		if _, err := Open(context.Background(), options(t), good, []byte("not-the-password")); !errors.Is(err, ErrWrongPassword) {
			t.Fatal(err)
		}
		idle(t)
	})
	t.Run("long password", func(t *testing.T) {
		if _, err := Open(context.Background(), options(t), good, make([]byte, MaxPasswordBytes+1)); !errors.Is(err, ErrLimit) {
			t.Fatal(err)
		}
	})
}

func encodeNoMAC(t *testing.T, key crypto.Signer, cert *x509.Certificate) []byte {
	raw, err := pkcs12.Passwordless.Encode(key, cert, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestWorkerDeadlinesAndCancellation(t *testing.T) {
	key := testKey(t, "RSA")
	cert := testCertificate(t, key)
	file := encode(t, pkcs12.Modern2023, key, cert, nil)
	// quick bounds the time from a deadline or cancellation to Open's
	// return: the worker is killed, not awaited.
	const quick = 2 * time.Second

	t.Run("load deadline", func(t *testing.T) {
		opt := fake(t, "slow")
		opt.LoadTimeout = 300 * time.Millisecond
		started := time.Now()
		if _, err := Open(context.Background(), opt, file, []byte(testPassword)); !errors.Is(err, ErrTimeout) {
			t.Fatal(err)
		}
		if time.Since(started) > quick {
			t.Fatal("the worker outlived its deadline")
		}
		idle(t)
	})
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(300*time.Millisecond, cancel)
		started := time.Now()
		if _, err := Open(ctx, fake(t, "slow"), file, []byte(testPassword)); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if time.Since(started) > quick {
			t.Fatal("the worker outlived the cancellation")
		}
		idle(t)
	})
	slow := file
	t.Run("hanging worker", func(t *testing.T) {
		opt := fake(t, "hang")
		opt.LoadTimeout = 200 * time.Millisecond
		if _, err := Open(context.Background(), opt, slow, []byte(testPassword)); !errors.Is(err, ErrTimeout) {
			t.Fatal(err)
		}
		idle(t)
	})
	t.Run("stalled signature", func(t *testing.T) {
		opt := fake(t, "stall")
		opt.SignTimeout = 200 * time.Millisecond
		file := encode(t, pkcs12.Modern2023, key, cert, nil)
		cred, err := Open(context.Background(), opt, file, []byte(testPassword))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte("x"))
		if _, err := cred.Sign(nil, digest[:], crypto.SHA256); !errors.Is(err, ErrTimeout) {
			t.Fatal(err)
		}
		if _, err := cred.Sign(nil, digest[:], crypto.SHA256); !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
		idle(t)
	})
	t.Run("worker exits", func(t *testing.T) {
		// A credential that is never closed does not keep its slot once
		// its worker has ended.
		cred, err := Open(context.Background(), fake(t, "exit"), file, []byte(testPassword))
		if err != nil {
			t.Fatal(err)
		}
		<-cred.done
		for deadline := time.Now().Add(time.Second); len(workers) != 0 && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
		idle(t)
		digest := sha256.Sum256([]byte("x"))
		if _, err := cred.Sign(nil, digest[:], crypto.SHA256); !errors.Is(err, ErrWorker) {
			t.Fatal(err)
		}
		idle(t)
	})
	t.Run("busy", func(t *testing.T) {
		file := encode(t, pkcs12.Modern2023, key, cert, nil)
		var open []*Credential
		for range maxWorkers {
			cred, err := Open(context.Background(), options(t), file, []byte(testPassword))
			if err != nil {
				t.Fatal(err)
			}
			open = append(open, cred)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if _, err := Open(ctx, options(t), file, []byte(testPassword)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		for _, c := range open {
			c.Close()
		}
		idle(t)
	})
}

func TestWorkerIdentityIsChecked(t *testing.T) {
	key := testKey(t, "ECDSA")
	cert := testCertificate(t, key)
	file := encode(t, pkcs12.Modern2023, key, cert, nil)
	for mode, want := range map[string]error{
		"nonce":       ErrWorker,
		"forge":       ErrCredential,
		"certificate": ErrCredential,
	} {
		t.Run(mode, func(t *testing.T) {
			if _, err := Open(context.Background(), fake(t, mode), file, []byte(testPassword)); !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			idle(t)
		})
	}
	t.Run("another program", func(t *testing.T) {
		cat, err := exec.LookPath("cat")
		if err != nil {
			t.Skip("no cat")
		}
		opt := options(t)
		opt.Executable, opt.args = cat, []string{}
		if _, err := Open(context.Background(), opt, file, []byte(testPassword)); !errors.Is(err, ErrWorker) {
			t.Fatal(err)
		}
		idle(t)
	})
	t.Run("missing executable", func(t *testing.T) {
		if _, err := Open(context.Background(), Options{}, file, []byte(testPassword)); !errors.Is(err, ErrWorker) {
			t.Fatal(err)
		}
		opt := options(t)
		opt.Executable = filepath.Join(t.TempDir(), "absent")
		if _, err := Open(context.Background(), opt, file, []byte(testPassword)); !errors.Is(err, ErrWorker) {
			t.Fatal(err)
		}
		idle(t)
	})
}

func TestSignRefusesOtherRequests(t *testing.T) {
	key := testKey(t, "RSA")
	cert := testCertificate(t, key)
	cred, err := Open(context.Background(), options(t), encode(t, pkcs12.Modern2026, key, cert, nil), []byte(testPassword))
	if err != nil {
		t.Fatal(err)
	}
	defer cred.Close()
	digest := sha256.Sum256([]byte("x"))
	for name, try := range map[string]func() ([]byte, error){
		"SHA-384":   func() ([]byte, error) { return cred.Sign(nil, make([]byte, 48), crypto.SHA384) },
		"PSS":       func() ([]byte, error) { return cred.Sign(nil, digest[:], &rsa.PSSOptions{Hash: crypto.SHA256}) },
		"no opts":   func() ([]byte, error) { return cred.Sign(nil, digest[:], nil) },
		"long data": func() ([]byte, error) { return cred.Sign(nil, make([]byte, 64), crypto.SHA256) },
	} {
		if _, err := try(); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	sig, err := cred.Sign(nil, digest[:], crypto.SHA256)
	if err != nil || rsa.VerifyPKCS1v15(cert.PublicKey.(*rsa.PublicKey), crypto.SHA256, digest[:], sig) != nil {
		t.Fatal(err)
	}
}

// TestWorkerSeesNoSecrets reads the worker's arguments and environment as
// the operating system shows them to other processes.
func TestWorkerSeesNoSecrets(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	key := testKey(t, "ECDSA")
	cert := testCertificate(t, key)
	cred, err := Open(context.Background(), options(t), encode(t, pkcs12.Modern2023, key, cert, nil), []byte(testPassword))
	if err != nil {
		t.Fatal(err)
	}
	defer cred.Close()
	proc := filepath.Join("/proc", strconv.Itoa(cred.cmd.Process.Pid))
	cmdline, err := os.ReadFile(filepath.Join(proc, "cmdline"))
	if err != nil {
		t.Fatal(err)
	}
	if args := strings.Split(strings.TrimSuffix(string(cmdline), "\x00"), "\x00"); len(args) != 2 || args[1] != WorkerFlag {
		t.Fatalf("worker arguments %q", args)
	}
	if env, err := os.ReadFile(filepath.Join(proc, "environ")); err != nil || len(env) != 0 {
		t.Fatalf("worker environment %q %v", env, err)
	}
	fds, err := os.ReadDir(filepath.Join(proc, "fd"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range fds {
		target, _ := os.Readlink(filepath.Join(proc, "fd", fd.Name()))
		// The runtime keeps the cgroup CPU quota open to follow it.
		if !strings.HasPrefix(target, "pipe:") && !strings.HasPrefix(target, "anon_inode:") && target != os.DevNull && !strings.HasPrefix(target, "/sys/fs/cgroup/") {
			t.Fatalf("worker holds %s", target)
		}
	}
}

func FuzzPreflight(f *testing.F) {
	key := mustEC()
	cert := testCertificate(nil, key)
	for _, enc := range []*pkcs12.Encoder{pkcs12.Modern2023, pkcs12.Modern2026, pkcs12.LegacyDES, pkcs12.LegacyRC2} {
		raw, err := enc.Encode(key, cert, nil, testPassword)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		cost, err := Preflight(raw)
		if err == nil && (cost.Iterations > MaxTotalIterations || cost.Derivations == 0) {
			t.Fatalf("accepted cost %+v", cost)
		}
	})
}

// TestBusyParentReadsTheLastFrame opens wrong-password files while other
// goroutines keep the parent busy: the worker's error frame, written just
// before it exits, must still be read.
func TestBusyParentReadsTheLastFrame(t *testing.T) {
	key := testKey(t, "ECDSA")
	file := encode(t, pkcs12.Modern2023, key, testCertificate(t, key), nil)
	stop := make(chan struct{})
	defer close(stop)
	for range 2 * runtime.GOMAXPROCS(0) {
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
					sha256.Sum256(file)
				}
			}
		}()
	}
	for range 40 {
		if _, err := Open(context.Background(), options(t), file, []byte("not-the-password")); !errors.Is(err, ErrWrongPassword) {
			t.Fatal(err)
		}
	}
	idle(t)
}

// TestDerivationWork counts the decoder's work per hash output block.
func TestDerivationWork(t *testing.T) {
	key := testKey(t, "ECDSA")
	cert := testCertificate(t, key)
	for name, c := range map[string]struct {
		enc  *pkcs12.Encoder
		want int64
	}{
		// MAC, key and certificates: one SHA-256 block each.
		"Modern2023": {pkcs12.Modern2023.WithIterations(1000), 3000},
		// MAC one SHA-1 block; key and certificates with 3DES: a two-block
		// key and a one-block IV each.
		"LegacyDES": {pkcs12.LegacyDES.WithIterations(1000), 7000},
		// The certificates use RC2-40: one block for the key, one for the IV.
		"LegacyRC2": {pkcs12.LegacyRC2.WithIterations(1000), 6000},
	} {
		cost, err := Preflight(encode(t, c.enc, key, cert, nil))
		if err != nil || cost.Iterations != c.want {
			t.Fatalf("%s: %+v %v, want %d", name, cost, err, c.want)
		}
	}
	cost := Cost{Iterations: MaxTotalIterations - 500, macWork: 1000}
	if err := cost.EmptyPassword(); !errors.Is(err, ErrLimit) {
		t.Fatal("the repeated MAC check is not counted:", err)
	}
	empty, err := pkcs12.Modern2023.Encode(key, cert, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	cred, err := Open(context.Background(), options(t), empty, nil)
	if err != nil {
		t.Fatal("empty password:", err)
	}
	cred.Close()
	idle(t)
}
