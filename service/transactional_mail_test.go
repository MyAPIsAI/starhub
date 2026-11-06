package service

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"starhub/model"
	"strings"
	"testing"
	"time"
)

func TestTransactionalMailSMTPUsesAccountLanguage(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	captured := make(chan string, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprint(connection, "220 fixture ESMTP\r\n")
		scanner := bufio.NewScanner(connection)
		data := false
		lines := []string{}
		for scanner.Scan() {
			line := scanner.Text()
			if data {
				if line == "." {
					captured <- strings.Join(lines, "\n")
					data = false
					fmt.Fprint(connection, "250 accepted\r\n")
				} else {
					lines = append(lines, line)
				}
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				fmt.Fprint(connection, "250 fixture\r\n")
			case line == "DATA":
				data = true
				fmt.Fprint(connection, "354 continue\r\n")
			case line == "QUIT":
				fmt.Fprint(connection, "221 done\r\n")
				return
			default:
				fmt.Fprint(connection, "250 ok\r\n")
			}
		}
	}()
	settings := map[string]string{"STARHUB_SMTP_ADDRESS": listener.Addr().String(), "STARHUB_SMTP_FROM": "notice@example.test"}
	sender, err := TransactionalMailSender(func(key string) string { return settings[key] })
	if err != nil {
		t.Fatal(err)
	}
	if err := sender(context.Background(), model.TransactionalMail{ID: 42, Recipient: "customer@example.test", Language: "zh", Kind: "password_changed"}); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-captured:
		if !strings.Contains(value, "Content-Language: zh") || !strings.Contains(value, "密码") || !strings.Contains(value, "starhub-notice-42@example.test") {
			t.Fatal("localized MIME message incomplete")
		}
	case <-time.After(time.Second):
		t.Fatal("SMTP capture missing")
	}
	for _, lang := range []string{"en", "zh", "ar"} {
		subject, body, err := model.RenderTransactionalMail("email_changed", lang)
		if err != nil || subject == "" || body == "" {
			t.Fatal("missing translation")
		}
	}
}
