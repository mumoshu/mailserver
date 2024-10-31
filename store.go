package main

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// store is a simple in-memory store for emails.
// It is used to store emails received by the SMTP server,
// and to store emails requested by the IMAP server.
type store struct {
	inboxes map[string]*inbox
}

type inbox struct {
	user   *imapmemserver.User
	emails []*email
}

// email represents an email message.
type email struct {
	from string
	to   string
	data string
}

// newStore creates a new store.
func newStore() *store {
	return &store{
		inboxes: make(map[string]*inbox),
	}
}

// Add adds an email to the store.
func (s *store) Add(e *email) {
	inbox, ok := s.inboxes[e.to]
	if !ok {
		panic(fmt.Sprintf("unknown inbox %v", e.to))
	}

	inbox.emails = append(inbox.emails, e)

	literalReader := strings.NewReader(e.data)
	inbox.user.Append("INBOX", literalReader, &imap.AppendOptions{})
}

// List returns all emails in an inbox.
func (s *store) List(mailAddr string) []*email {
	inbox, ok := s.inboxes[mailAddr]
	if !ok {
		panic(fmt.Sprintf("unknown inbox %v", inbox))
	}

	return inbox.emails
}

func (s *store) AddUser(mailAddr string, user *imapmemserver.User) {
	s.inboxes[mailAddr] = &inbox{
		user:   user,
		emails: make([]*email, 0),
	}
}
