// Package mailer 提供最小可用的 SMTP 发信能力：组装 UTF-8 MIME 邮件，
// 支持 starttls（587）/ implicit TLS（465）/ none（本机中继）三种连接方式。
//
// 设计背景与取舍见 docs/auth-email-verification-design.md §3.1 / §4。
package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// TLS 连接方式。
const (
	// TLSStartTLS 先明文连接再用 STARTTLS 升级（默认，587 常用）。
	TLSStartTLS = "starttls"
	// TLSImplicit 连接即 TLS（465 常用）。
	TLSImplicit = "implicit"
	// TLSNone 不加密，仅允许无账号密码的本机/内网中继。
	TLSNone = "none"
)

const (
	defaultPort  = 587
	implicitPort = 465
	// sendTimeout 是单封邮件从连接建立到 QUIT 的总超时上限。
	sendTimeout = 15 * time.Second
	// maxHeaderLine 是 base64 正文的折行宽度（RFC 2045 建议不超过 76）。
	maxBodyLine = 76
)

// Config 是 SMTP 发信配置，同时用于 manager.yaml 的 smtp 段
// （docs/auth-email-verification-design.md §3.1）。
type Config struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"` // 0 = 按 tls 方式取默认端口（starttls 587 / implicit 465）
	// TLS 连接方式：starttls（默认）| implicit（465 直连 TLS）| none（仅本机/内网中继，
	// 不允许带账号密码）。
	TLS      string `yaml:"tls"`
	Username string `yaml:"username"`
	Password string `yaml:"password"` // 敏感值：注意配置文件权限，不要提交到仓库
	From     string `yaml:"from"`     // 发件人，如 "Orion-X <no-reply@example.com>"
}

// Mailer 按固定配置发送纯文本 UTF-8 邮件，可并发使用。
type Mailer struct {
	host     string
	port     int
	tlsMode  string
	username string
	password string
	from     *mail.Address
	// tlsConfig 仅测试注入 RootCAs 用；nil = 默认校验系统根证书。
	tlsConfig *tls.Config
}

// New 校验配置并返回 Mailer：smtp.host 为空视为未配置，返回 (nil, nil)；其余
// 配置错误在启动期暴露给运维，而不是等第一封邮件。
func New(cfg Config) (*Mailer, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return nil, nil
	}

	tlsMode := strings.ToLower(strings.TrimSpace(cfg.TLS))
	if tlsMode == "" {
		tlsMode = TLSStartTLS
	}
	switch tlsMode {
	case TLSStartTLS, TLSImplicit, TLSNone:
	default:
		return nil, fmt.Errorf("mailer: unknown tls mode %q (want %s | %s | %s)",
			cfg.TLS, TLSStartTLS, TLSImplicit, TLSNone)
	}

	port := cfg.Port
	if port == 0 {
		port = defaultPort
		if tlsMode == TLSImplicit {
			port = implicitPort
		}
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("mailer: invalid port %d", port)
	}

	username := strings.TrimSpace(cfg.Username)
	if tlsMode == TLSNone && username != "" && !isLocalhost(host) {
		// 明文链路上禁止发凭据（本机中继例外，与 net/smtp 的 PlainAuth 规则一致）。
		return nil, fmt.Errorf("mailer: credentials over tls none are only allowed on localhost, got host %q", host)
	}

	from, err := mail.ParseAddress(strings.TrimSpace(cfg.From))
	if err != nil {
		return nil, fmt.Errorf("mailer: invalid from address %q: %w", cfg.From, err)
	}

	return &Mailer{
		host:     host,
		port:     port,
		tlsMode:  tlsMode,
		username: username,
		password: cfg.Password,
		from:     from,
	}, nil
}

// Send 向 to 发送一封纯文本邮件。ctx 取消或超时会中断发送。
func (m *Mailer) Send(ctx context.Context, to, subject, body string) error {
	recipient, err := mail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return fmt.Errorf("mailer: invalid recipient %q: %w", to, err)
	}
	msg, err := buildMessage(m.from, recipient, subject, body)
	if err != nil {
		return err
	}

	conn, err := m.dial(ctx)
	if err != nil {
		return fmt.Errorf("mailer: dial %s: %w", m.addr(), err)
	}
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(sendTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("mailer: set deadline: %w", err)
	}

	return m.send(conn, recipient.Address, msg)
}

func (m *Mailer) addr() string { return net.JoinHostPort(m.host, fmt.Sprintf("%d", m.port)) }

func (m *Mailer) dial(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{}
	if m.tlsMode == TLSImplicit {
		return (&tls.Dialer{NetDialer: dialer, Config: m.tlsConfigFor()}).DialContext(ctx, "tcp", m.addr())
	}
	return dialer.DialContext(ctx, "tcp", m.addr())
}

func (m *Mailer) tlsConfigFor() *tls.Config {
	if m.tlsConfig != nil {
		return m.tlsConfig
	}
	return &tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12}
}

func (m *Mailer) send(conn net.Conn, recipient string, msg []byte) error {
	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return fmt.Errorf("mailer: greeting: %w", err)
	}
	defer func() { _ = c.Close() }()

	if m.tlsMode == TLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("mailer: server does not advertise STARTTLS")
		}
		if err := c.StartTLS(m.tlsConfigFor()); err != nil {
			return fmt.Errorf("mailer: starttls: %w", err)
		}
	}

	if m.username != "" {
		auth, err := m.pickAuth(c)
		if err != nil {
			return err
		}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("mailer: auth: %w", err)
		}
	}

	if err := c.Mail(m.from.Address); err != nil {
		return fmt.Errorf("mailer: mail from: %w", err)
	}
	if err := c.Rcpt(recipient); err != nil {
		return fmt.Errorf("mailer: rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: data: %w", err)
	}
	_, writeErr := w.Write(msg)
	closeErr := w.Close()
	if writeErr != nil {
		return fmt.Errorf("mailer: write body: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("mailer: finish body: %w", closeErr)
	}
	if err := c.Quit(); err != nil {
		return fmt.Errorf("mailer: quit: %w", err)
	}
	return nil
}

// pickAuth 按服务端声明的机制选择认证方式。net/smtp 只内置 PLAIN 与
// CRAM-MD5，部分服务端（如国内邮箱的 SMTP）只声明 LOGIN，需要自实现。
func (m *Mailer) pickAuth(c *smtp.Client) (smtp.Auth, error) {
	var mechanisms string
	if ok, params := c.Extension("AUTH"); ok {
		mechanisms = strings.ToUpper(params)
	}
	switch {
	case strings.Contains(mechanisms, "PLAIN"):
		return smtp.PlainAuth("", m.username, m.password, m.host), nil
	case strings.Contains(mechanisms, "LOGIN"):
		return &loginAuth{username: m.username, password: m.password}, nil
	case strings.Contains(mechanisms, "CRAM-MD5"):
		return smtp.CRAMMD5Auth(m.username, m.password), nil
	default:
		return nil, fmt.Errorf("mailer: server advertises no supported auth mechanism (got %q)", mechanisms)
	}
}

// loginAuth 实现 SMTP AUTH LOGIN（RFC 未定义，但被广泛支持）：
// 先回 "LOGIN"，再按服务端挑战依次提交用户名、密码。
type loginAuth struct{ username, password string }

func (a *loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) { return "LOGIN", nil, nil }

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
	case "username:", "user name:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("mailer: unexpected LOGIN challenge %q", fromServer)
}

// buildMessage 组装邮件。主题用 MIME encoded-word，正文 base64：
// 避免 8bit 正文被不支持 SMTPUTF8 的服务器改写。
func buildMessage(from, to *mail.Address, subject, body string) ([]byte, error) {
	if strings.ContainsAny(subject, "\r\n") {
		return nil, errors.New("mailer: subject must not contain CR/LF")
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from.String())
	fmt.Fprintf(&buf, "To: %s\r\n", to.String())
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString("\r\n")

	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	for len(encoded) > maxBodyLine {
		buf.WriteString(encoded[:maxBodyLine])
		buf.WriteString("\r\n")
		encoded = encoded[maxBodyLine:]
	}
	buf.WriteString(encoded)
	return buf.Bytes(), nil
}

func isLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
