package main

import (
	"io"
	"log"

	"github.com/emersion/go-smtp"
)

type smtpBackend struct {
	store *store
}

func (bkd *smtpBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &smtpSession{
		email:   &email{},
		backend: bkd,
	}, nil
}

type smtpSession struct {
	email   *email
	backend *smtpBackend
}

func (s *smtpSession) AuthPlain(username, password string) error {
	log.Printf("AuthPlain: %s", username)
	// TODO check username and password
	return nil
}

func (s *smtpSession) Mail(from string, opts *smtp.MailOptions) error {
	log.Printf("MAIL FROM: %s", from)
	if opts != nil {
		log.Printf("Mail options: %v", *opts)
	}

	s.email.from = from
	return nil
}

func (s *smtpSession) Rcpt(to string, opts *smtp.RcptOptions) error {
	log.Printf("RCPT TO: %s", to)
	if opts != nil {
		log.Printf("Rcpt options: %v", *opts)
	}

	s.email.to = to
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	log.Printf("DATA")

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	s.email.data = string(data)

	log.Printf("Received DATA: %s", s.email.data)

	s.backend.store.Add(s.email)

	return nil
}

func (s *smtpSession) Reset() {}

func (s *smtpSession) Logout() error {
	return nil
}
