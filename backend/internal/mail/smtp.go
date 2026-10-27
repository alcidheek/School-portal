package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"time"
)

type SMTPSender struct {
	host, port, email, password string
}

func NewSMTPSender(host, port, email, password string) (*SMTPSender, error) {
	n, err := strconv.Atoi(port)
	if host == "" || err != nil || n < 1 || n > 65535 || password == "" {
		return nil, fmt.Errorf("SMTP_HOST, SMTP_PORT, SMTP_EMAIL and SMTP_APP_PASSWORD must be configured")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return nil, fmt.Errorf("SMTP_EMAIL must be a valid email address")
	}
	return &SMTPSender{host: host, port: port, email: email, password: password}, nil
}

func (s *SMTPSender) SendResetCode(ctx context.Context, email, code string) error {
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return fmt.Errorf("invalid recipient email")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.host, s.port))
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	tlsConfig := &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}
	// Gmail supports implicit TLS on 465 and STARTTLS on 587.
	var smtpConn net.Conn = conn
	if s.port == "465" {
		smtpConn = tls.Client(conn, tlsConfig)
	}
	client, err := smtp.NewClient(smtpConn, s.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if s.port != "465" {
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if err := client.Auth(smtp.PlainAuth("", s.email, s.password, s.host)); err != nil {
		return err
	}
	if err := client.Mail(s.email); err != nil {
		return err
	}
	if err := client.Rcpt(email); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: School portal password reset\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nYour password reset code is: %s\r\nThis code expires in 15 minutes.\r\nIf you did not request this, ignore this email.\r\n", s.email, email, code)
	if _, err := w.Write([]byte(message)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	// DATA acceptance confirms delivery to the SMTP server.
	_ = client.Quit()
	return nil
}
