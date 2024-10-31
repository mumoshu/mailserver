package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"crypto/tls"
	"io"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/emersion/go-smtp"
)

func main() {
	ctx := signalingContext(context.Background())
	if err := run(ctx, os.Args[0], os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func signalingContext(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		<-sigCh
		cancel()
	}()
	return ctx
}

type settings struct {
	smtpAddr  string
	imapAddr  string
	imapsAddr string

	tlsCert  string
	tlsKey   string
	username string
	password string

	domain string

	insecureAuth bool
	debug        bool
}

func (c *settings) defineFlags(flagSet *flag.FlagSet) {
	// SMTP server configuration
	flagSet.StringVar(&c.smtpAddr, "stmp-listen", c.smtpAddr, "SMTP listening address")

	// IMAP server configuration
	flagSet.StringVar(&c.imapAddr, "imap-listen", c.imapAddr, "IMAP listening address")
	flagSet.StringVar(&c.imapsAddr, "imaps-listen", c.imapsAddr, "IMAPs listening address, usually :993")
	flagSet.StringVar(&c.tlsCert, "tls-cert", "", "TLS certificate")
	flagSet.StringVar(&c.tlsKey, "tls-key", "", "TLS key")
	flagSet.StringVar(&c.username, "username", "user", "Username")
	flagSet.StringVar(&c.password, "password", "user", "Password")

	// Common configuration
	flagSet.StringVar(&c.domain, "domain", "localhost", "Domain name")
	flagSet.BoolVar(&c.insecureAuth, "insecure-auth", false, "Allow authentication without TLS")
	flagSet.BoolVar(&c.debug, "debug", false, "Print all commands and responses")
}

type config struct {
	settings

	debugWriter io.Writer
}

// mailserver supports the following protocols:
// - SMTP
// - IMAP
// - IMAPs (IMAP over implicit TLS, usually :993)
func run(ctx context.Context, cmd string, args []string, debugOut io.Writer) error {
	ss := &settings{
		smtpAddr: "127.0.0.1:31025",
		imapAddr: "127.0.0.1:30143",
	}

	flagSet := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	ss.defineFlags(flagSet)
	if err := flagSet.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	c := &config{}

	var debugWriter io.Writer
	if ss.debug {
		c.debugWriter = debugOut
	}

	store := newStore()

	var smtpServer *smtp.Server
	{
		smtpServer = smtp.NewServer(&smtpBackend{
			store: store,
		})
		smtpServer.Addr = ss.smtpAddr
		smtpServer.Domain = "localhost"
		smtpServer.AllowInsecureAuth = ss.insecureAuth
		smtpServer.Debug = debugWriter
	}

	var imapServer *imapserver.Server
	{
		var tlsConfig *tls.Config
		if ss.tlsCert != "" || ss.tlsKey != "" {
			cert, err := tls.LoadX509KeyPair(ss.tlsCert, ss.tlsKey)
			if err != nil {
				log.Fatalf("Failed to load TLS key pair: %v", err)
			}
			tlsConfig = &tls.Config{
				Certificates: []tls.Certificate{cert},
			}
		}

		memServer := imapmemserver.New()

		if ss.username != "" || ss.password != "" {
			user := imapmemserver.NewUser(ss.username, ss.password)
			if err := user.Create("INBOX", nil); err != nil {
				return fmt.Errorf("failed to create user: %w", err)
			}
			memServer.AddUser(user)

			store.AddUser(fmt.Sprintf("%s@%s", ss.username, ss.domain), user)
		}

		imapServer = imapserver.New(&imapserver.Options{
			NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
				return memServer.NewSession(), nil, nil
			},
			Caps: imap.CapSet{
				imap.CapIMAP4rev1: {},
				imap.CapIMAP4rev2: {},
			},
			TLSConfig:    tlsConfig,
			InsecureAuth: ss.insecureAuth,
			DebugWriter:  debugWriter,
		})
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var errChs []chan error

	errSMTP := runAsync(ctx, "SMTP server", func() error {
		return smtpServer.ListenAndServe()
	}, func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return smtpServer.Shutdown(shutdownCtx)
	})
	errChs = append(errChs, errSMTP)

	errInsecureIMAP := runAsync(ctx, "IMAP serer", func() error {
		return imapServer.ListenAndServe(ss.imapAddr)
	}, func() error {
		return imapServer.Close()
	})
	errChs = append(errChs, errInsecureIMAP)

	if ss.imapsAddr != "" {
		errSecureIMAP := runAsync(ctx, "IMAPs server", func() error {
			return imapServer.ListenAndServeTLS(ss.imapsAddr)
		}, func() error {
			return imapServer.Close()
		})
		errChs = append(errChs, errSecureIMAP)
	}

	aggregaCh := make(chan error, len(errChs))
	for _, errCh := range errChs {
		go func(errCh chan error) {
			aggregaCh <- <-errCh
		}(errCh)
	}

	// Wait for all servers to stop
	var results []error
	want := len(errChs)
	for err := range aggregaCh {
		results = append(results, err)

		// Stop all servers if any of them stopped
		cancel()

		if len(results) == want {
			log.Println("All servers stopped")
			break
		} else if len(results) > want {
			panic(fmt.Sprintf("encountered more than %d results. Bug?", want))
		}
	}

	if len(results) > 0 {
		var diagnostics []string
		for _, err := range results {
			if err != nil {
				diagnostics = append(diagnostics, err.Error())
			}
		}
		if len(diagnostics) > 0 {
			return fmt.Errorf("errors encountered: %s", strings.Join(diagnostics, "\n"))
		}
	}

	return nil
}

func runAsync(ctx context.Context, name string, start, stop func() error) chan error {
	var err = make(chan error, 1)

	go func() {
		err <- cancellableRun(ctx, func() error {
			log.Printf("Starting %s", name)
			if err := start(); err != nil {
				return fmt.Errorf("failed to start %s: %w", name, err)
			}
			return nil
		}, func() error {
			log.Printf("Stopping %s", name)
			if err := stop(); err != nil {
				return fmt.Errorf("failed to stop %s: %w", name, err)
			}
			return nil
		})
	}()

	return err
}

func cancellableRun(ctx context.Context, f, stop func() error) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- f()
	}()

	select {
	case <-ctx.Done():
		return stop()
	case err := <-errCh:
		return err
	}
}
