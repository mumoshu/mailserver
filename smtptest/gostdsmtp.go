package smtptest

import (
	"fmt"
	"net/smtp"
	"strings"
)

type ClientConfig struct {
	Host string
	Port int

	Username, Password string
}

type Gostd struct {
	ClientConfig
}

func (g *Gostd) Sendmail(from string, recipients []string, subject, body string) error {
	var auth smtp.Auth
	if g.Username != "" && g.Password != "" {
		auth = smtp.CRAMMD5Auth(g.Username, g.Password)
	}
	msg := []byte(strings.ReplaceAll(fmt.Sprintf("To: %s\nSubject: %s\n\n%s", strings.Join(recipients, ","), subject, body), "\n", "\r\n"))
	return smtp.SendMail(fmt.Sprintf("%s:%d", g.Host, g.Port), auth, from, recipients, msg)
}
