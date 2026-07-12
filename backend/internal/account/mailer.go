package account

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Mailer delivers one transactional plain-text message.
type Mailer interface {
	Send(ctx context.Context, message EmailMessage) error
}

// SMTPConfig contains authenticated SMTP delivery settings.
type SMTPConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	ImplicitTLS bool
}

// SMTPMailer sends transactional mail with STARTTLS or implicit TLS.
type SMTPMailer struct {
	config SMTPConfig
}

// NewSMTPMailer validates settings and returns a bounded SMTP sender.
func NewSMTPMailer(config SMTPConfig) (*SMTPMailer, error) {
	config.Host = strings.TrimSpace(config.Host)
	config.FromAddress = strings.TrimSpace(config.FromAddress)
	if config.Host == "" || config.Port < 1 || config.FromAddress == "" {
		return nil, fmt.Errorf("smtp host, port, and from address are required")
	}
	return &SMTPMailer{config: config}, nil
}

// Send delivers a CRLF-normalized plain-text message.
func (m *SMTPMailer) Send(ctx context.Context, message EmailMessage) error {
	address := net.JoinHostPort(m.config.Host, fmt.Sprintf("%d", m.config.Port))
	dialer := net.Dialer{Timeout: 10 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer connection.Close()

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: m.config.Host}
	if m.config.ImplicitTLS {
		connection = tls.Client(connection, tlsConfig)
	}
	client, err := smtp.NewClient(connection, m.config.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if !m.config.ImplicitTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp server does not advertise STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if m.config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.config.Username, m.config.Password, m.config.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(m.config.FromAddress); err != nil {
		return err
	}
	if err := client.Rcpt(message.Recipient); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	buffered := bufio.NewWriter(writer)
	from := m.config.FromAddress
	if strings.TrimSpace(m.config.FromName) != "" {
		from = fmt.Sprintf("%s <%s>", strings.TrimSpace(m.config.FromName), m.config.FromAddress)
	}
	body := strings.ReplaceAll(message.TextBody, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	if _, err := fmt.Fprintf(buffered, "From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", from, message.Recipient, sanitizeHeader(message.Subject), body); err != nil {
		return err
	}
	if err := buffered.Flush(); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// sanitizeHeader prevents SMTP header injection through operator-controlled subjects.
func sanitizeHeader(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}
