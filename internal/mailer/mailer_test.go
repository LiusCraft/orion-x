package mailer

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestNewValidation(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "from required", cfg: Config{Host: "smtp.example.com"}, wantErr: "invalid from address"},
		{
			name:    "unknown tls mode",
			cfg:     Config{Host: "smtp.example.com", From: "a@example.com", TLS: "sslv3"},
			wantErr: "unknown tls mode",
		},
		{
			name:    "credentials need encryption off localhost",
			cfg:     Config{Host: "smtp.example.com", From: "a@example.com", TLS: TLSNone, Username: "u", Password: "p"},
			wantErr: "only allowed on localhost",
		},
		{
			name:    "invalid port",
			cfg:     Config{Host: "smtp.example.com", From: "a@example.com", Port: 70000},
			wantErr: "invalid port",
		},
		{
			name: "localhost relay with credentials is allowed",
			cfg:  Config{Host: "127.0.0.1", From: "a@example.com", TLS: TLSNone, Username: "u", Password: "p"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("New() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("New() error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestNewUnconfigured(t *testing.T) {
	m, err := New(Config{})
	if err != nil {
		t.Fatalf("New(Config{}) error = %v, want nil", err)
	}
	if m != nil {
		t.Fatalf("New(Config{}) mailer = %v, want nil (未配置 smtp)", m)
	}
}

func TestNewDefaultPorts(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want int
	}{
		{name: "starttls default", cfg: Config{Host: "smtp.example.com", From: "a@example.com"}, want: 587},
		{name: "implicit default", cfg: Config{Host: "smtp.example.com", From: "a@example.com", TLS: TLSImplicit}, want: 465},
		{name: "explicit port wins", cfg: Config{Host: "smtp.example.com", From: "a@example.com", Port: 2525}, want: 2525},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := New(tc.cfg)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if m.port != tc.want {
				t.Errorf("port = %d, want %d", m.port, tc.want)
			}
		})
	}
}

func TestSendWithoutAuth(t *testing.T) {
	srv := startFakeServer(t, fakeOptions{})
	m, err := New(Config{Host: "127.0.0.1", Port: srv.port(), TLS: TLSNone, From: "Orion-X <no-reply@example.com>"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := m.Send(context.Background(), "user@example.com", "验证你的邮箱", "hello 世界"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	msg := srv.receive(t)

	if !strings.Contains(msg, "To: <user@example.com>") {
		t.Errorf("message missing To header: %q", msg)
	}
	if !strings.Contains(msg, "Subject: =?utf-8?") {
		t.Errorf("subject must be MIME encoded: %q", msg)
	}
	if got := decodeBody(t, msg); got != "hello 世界" {
		t.Errorf("body = %q, want %q", got, "hello 世界")
	}
}

func TestSendLoginAuth(t *testing.T) {
	srv := startFakeServer(t, fakeOptions{authMechs: []string{"LOGIN"}, username: "smtp-user", password: "smtp-pass"})
	m, err := New(Config{
		Host: "127.0.0.1", Port: srv.port(), TLS: TLSNone,
		Username: "smtp-user", Password: "smtp-pass", From: "a@example.com",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := m.Send(context.Background(), "user@example.com", "hello", "body"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	srv.receive(t)
}

func TestSendStartTLSPlainAuth(t *testing.T) {
	cert, pool := selfSignedCert(t)
	srv := startFakeServer(t, fakeOptions{
		startTLS:  true,
		tlsConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
		authMechs: []string{"PLAIN"},
		username:  "smtp-user",
		password:  "smtp-pass",
	})
	m, err := New(Config{
		Host: "127.0.0.1", Port: srv.port(), TLS: TLSStartTLS,
		Username: "smtp-user", Password: "smtp-pass", From: "a@example.com",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	m.tlsConfig = &tls.Config{ServerName: "127.0.0.1", RootCAs: pool, MinVersion: tls.VersionTLS12}

	if err := m.Send(context.Background(), "user@example.com", "hello", "body"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	srv.receive(t)
}

func TestSendRejectsBadRecipient(t *testing.T) {
	m, err := New(Config{Host: "127.0.0.1", TLS: TLSNone, From: "a@example.com"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := m.Send(context.Background(), "not an address", "s", "b"); err == nil {
		t.Fatal("Send() error = nil, want invalid recipient error")
	}
}

func TestBuildMessageRejectsHeaderInjection(t *testing.T) {
	from := &mail.Address{Address: "from@example.com"}
	to := &mail.Address{Address: "to@example.com"}
	if _, err := buildMessage(from, to, "hello\r\nBcc: evil@example.com", "body"); err == nil {
		t.Fatal("buildMessage() error = nil, want CR/LF in subject rejected")
	}
}

// ---------------------------------------------------------------------------
// 测试用假 SMTP 服务器
// ---------------------------------------------------------------------------

type fakeOptions struct {
	startTLS  bool
	tlsConfig *tls.Config
	authMechs []string
	username  string
	password  string
}

type fakeServer struct {
	t        *testing.T
	ln       net.Listener
	opts     fakeOptions
	messages chan string
}

func startFakeServer(t *testing.T, opts fakeOptions) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &fakeServer{t: t, ln: ln, opts: opts, messages: make(chan string, 1)}
	go srv.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return srv
}

func (f *fakeServer) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeServer) receive(t *testing.T) string {
	t.Helper()
	select {
	case msg := <-f.messages:
		return msg
	case <-time.After(3 * time.Second):
		t.Fatal("fake SMTP server did not receive a message")
		return ""
	}
}

func (f *fakeServer) serve() {
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	r := bufio.NewReader(conn)
	send := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	send("220 fake ESMTP ready")

	var (
		inData    bool
		data      strings.Builder
		loginStep int // 1 = username, 2 = password
		authUser  string
	)

	for {
		raw, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line := strings.TrimRight(raw, "\r\n")

		if inData {
			if line == "." {
				inData = false
				f.messages <- data.String()
				send("250 Ok: queued")
				continue
			}
			data.WriteString(line + "\r\n")
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case loginStep == 1:
			decoded, _ := base64.StdEncoding.DecodeString(line)
			authUser = string(decoded)
			loginStep = 2
			send("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
		case loginStep == 2:
			decoded, _ := base64.StdEncoding.DecodeString(line)
			loginStep = 0
			if authUser == f.opts.username && string(decoded) == f.opts.password {
				send("235 authenticated")
			} else {
				send("535 bad credentials")
			}
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			send("250-fake")
			if f.opts.startTLS {
				send("250-STARTTLS")
			}
			if len(f.opts.authMechs) > 0 {
				send("250-AUTH " + strings.Join(f.opts.authMechs, " "))
			}
			send("250 OK")
		case upper == "STARTTLS" && f.opts.startTLS:
			send("220 Ready to start TLS")
			tlsConn := tls.Server(conn, f.opts.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			r = bufio.NewReader(conn)
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			parts := strings.Fields(line)
			payload := ""
			if len(parts) > 2 {
				payload = parts[2]
			}
			decoded, _ := base64.StdEncoding.DecodeString(payload)
			segs := strings.Split(string(decoded), "\x00")
			if len(segs) == 3 && segs[1] == f.opts.username && segs[2] == f.opts.password {
				send("235 authenticated")
			} else {
				send("535 bad credentials")
			}
		case upper == "AUTH LOGIN":
			loginStep = 1
			send("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
		case strings.HasPrefix(upper, "MAIL FROM:"), strings.HasPrefix(upper, "RCPT TO:"):
			send("250 OK")
		case upper == "DATA":
			inData = true
			send("354 End data with <CR><LF>.<CR><LF>")
		case upper == "QUIT":
			send("221 Bye")
			return
		default:
			send("250 OK")
		}
	}
}

func decodeBody(t *testing.T, msg string) string {
	t.Helper()
	_, body, ok := strings.Cut(msg, "\r\n\r\n")
	if !ok {
		t.Fatalf("message has no header/body separator: %q", msg)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", ""))
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return string(decoded)
}

func selfSignedCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("key pair: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)
	return cert, pool
}
