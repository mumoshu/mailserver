package imaptest

import (
	"context"
	"fmt"
	"log"
	"mime"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
)

type Emersion struct {
	Addr     string
	User     string
	Password string
}

func (e *Emersion) FetchFirstMessage(ctx context.Context) (*imapclient.FetchMessageBuffer, error) {
	opts := &imapclient.Options{
		// Use go-message's collection of charset readers to decode messages
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
	}

	log.Printf("Connecting to %v", e.Addr)

	c, err := imapclient.DialInsecure(e.Addr, opts)
	if err != nil {
		return nil, err
	}

	log.Printf("Connected to %v", e.Addr)

	if _, err := run(ctx, func() (interface{}, error) {
		if err := c.Login(e.User, e.Password).Wait(); err != nil {
			return nil, fmt.Errorf("failed to login: %v", err)
		}

		return nil, nil
	}); err != nil {
		return nil, err
	}

	mailboxes, err := c.List("", "%", nil).Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to list mailboxes: %v", err)
	}

	log.Printf("Found %v mailboxes", len(mailboxes))
	for _, mbox := range mailboxes {
		log.Printf(" - %v", mbox.Mailbox)
	}

	// Select the mailbox you want to read
	mailbox, err := c.Select("INBOX", nil).Wait()
	if err != nil {
		return nil, err
	}
	log.Printf("INBOX contains %v messages", mailbox.NumMessages)

	var message *imapclient.FetchMessageBuffer
	if mailbox.NumMessages > 0 {
		seqSet := imap.SeqSetNum(1)
		fetchOptions := &imap.FetchOptions{
			Envelope: true,

			// To fetch the entire body...
			BodySection: []*imap.FetchItemBodySection{
				{},
			},
		}
		messages, err := c.Fetch(seqSet, fetchOptions).Collect()
		if err != nil {
			log.Fatalf("failed to fetch first message in INBOX: %v", err)
		}
		log.Printf("subject of first message in INBOX: %v", messages[0].Envelope.Subject)
		message = messages[0]
	}

	if err := c.Logout().Wait(); err != nil {
		return nil, fmt.Errorf("failed to logout: %v", err)
	}

	return message, nil
}

func run(ctx context.Context, f func() (interface{}, error)) (interface{}, error) {
	type res struct {
		v   interface{}
		err error
	}

	ch := make(chan res)
	defer close(ch)
	go func() {
		v, err := f()
		ch <- res{v, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.v, r.err
	}
}
