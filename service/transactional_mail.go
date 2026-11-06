package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"starhub/model"
	"strings"
	"time"
)

// SMTP is optional. Credentials are read only from the process environment.
func TransactionalMailSender(getenv func(string) string) (func(context.Context, model.TransactionalMail) error, error) {
	address, from := getenv("STARHUB_SMTP_ADDRESS"), getenv("STARHUB_SMTP_FROM")
	if address == "" && from == "" {
		return nil, nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP address")
	}
	parsed, err := mail.ParseAddress(from)
	if err != nil || parsed.Address != from {
		return nil, fmt.Errorf("invalid SMTP sender")
	}
	username, password := getenv("STARHUB_SMTP_USERNAME"), getenv("STARHUB_SMTP_PASSWORD")
	return func(ctx context.Context, row model.TransactionalMail) error {
		recipient, err := mail.ParseAddress(row.Recipient)
		if err != nil || recipient.Address != row.Recipient {
			return fmt.Errorf("invalid mail recipient")
		}
		subject, body, err := model.RenderTransactionalMail(row.Kind, row.Language)
		if err != nil {
			return err
		}
		connection, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
		if err != nil {
			return fmt.Errorf("mail connection unavailable")
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(20 * time.Second))
		client, err := smtp.NewClient(connection, host)
		if err != nil {
			return fmt.Errorf("mail greeting failed")
		}
		defer client.Close()
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("mail TLS failed")
			}
		} else if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("mail server requires TLS")
		}
		if username != "" {
			if err := client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
				return fmt.Errorf("mail authentication failed")
			}
		}
		if err := client.Mail(from); err != nil {
			return fmt.Errorf("mail sender rejected")
		}
		if err := client.Rcpt(row.Recipient); err != nil {
			return fmt.Errorf("mail recipient rejected")
		}
		writer, err := client.Data()
		if err != nil {
			return fmt.Errorf("mail delivery unavailable")
		}
		message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <starhub-notice-%d@%s>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\nContent-Language: %s\r\n\r\n%s\r\n", from, row.Recipient, mime.QEncoding.Encode("UTF-8", subject), row.ID, strings.Split(from, "@")[1], row.Language, body)
		if _, err := writer.Write([]byte(message)); err != nil {
			return fmt.Errorf("mail write failed")
		}
		if err := writer.Close(); err != nil {
			return fmt.Errorf("mail acknowledgement failed")
		}
		return client.Quit()
	}, nil
}

func WatchTransactionalMail(ctx context.Context) error {
	send, err := TransactionalMailSender(os.Getenv)
	if err != nil || send == nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				_ = model.DeliverTransactionalMail(ctx, now, send)
			}
		}
	}()
	return nil
}
