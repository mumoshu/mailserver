package main

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/mumoshu/mailserver/imaptest"
	"github.com/mumoshu/mailserver/smtptest"
	"github.com/stretchr/testify/assert"
)

type debugWriter struct {
	fn func(args ...interface{})
}

func (w *debugWriter) Write(p []byte) (n int, err error) {
	w.fn(string(p))
	return len(p), nil
}

func TestRun(t *testing.T) {
	var debugWriter io.Writer = &debugWriter{fn: t.Log}

	var (
		cmd  = "mailserver"
		args = []string{
			"-stmp-listen", "localhost:31025",
			"-imap-listen", "127.0.0.1:30143",
			"-username", "user",
			"-password", "pass",
			"-domain", "example.com",
			"-insecure-auth",
		}
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := run(ctx, cmd, args, debugWriter); err != nil {
			log.Printf("failed to run: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)

	smtpClient := &smtptest.Gostd{
		ClientConfig: smtptest.ClientConfig{
			Host: "localhost",
			Port: 31025,
			// Username: "user",
			// Password: "user",
		},
	}

	err := smtpClient.Sendmail("foo@example.com", []string{"user@example.com"}, "Hello", "World")
	if err != nil {
		t.Error(err)
	}

	imapClient := imaptest.Emersion{
		User:     "user",
		Password: "pass",
		Addr:     "localhost:30143",
	}

	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer fetchCancel()

	msg, err := imapClient.FetchFirstMessage(fetchCtx)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Fetched message envelope: %v", *msg.Envelope)

	if msg.Envelope.Subject != "Hello" {
		t.Errorf("unexpected subject: %v", msg.Envelope.Subject)
	}

	if msg.Envelope.To[0].Mailbox != "user" {
		t.Errorf("unexpected to mailbox: %v", msg.Envelope.To[0].Mailbox)
	}

	if msg.Envelope.To[0].Host != "example.com" {
		t.Errorf("unexpected to host: %v", msg.Envelope.To[0].Host)
	}

	if len(msg.BodySection) == 0 {
		t.Errorf("no body section found")
	}

	for _, s := range msg.BodySection {
		// We need to normalize the line endings to compare the body
		body := string(s)
		body = strings.ReplaceAll(body, "\r\n", "\n")

		assert.Equal(t, `To: user@example.com
Subject: Hello

World
`, body)
	}

	if msg.BodyStructure != nil {
		msg.BodyStructure.Walk(func(path []int, part imap.BodyStructure) bool {
			switch p := part.(type) {
			case *imap.BodyStructureSinglePart:
				log.Printf("BodyPart: %v", p)
			case *imap.BodyStructureMultiPart:
				log.Printf("BodyPart: %v", p)
			}
			return true
		})
	}
}
