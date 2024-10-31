# mailserver

This is a demo project to show how to use the emmersion `go-smtp` and `go-imap` packages.

To do so, we have a server application that integrates:

- SMTP server
- IMAP server
- In-memory storage, connected to the SMTP and IMAP servers

In theory, you could use this as a starting point for a full email server.

For example, to send an email over AWS SES, you could modify the in-memory storage to just forward the emails destined for the outside world to SES.

Another example would be to receive emails from SES-written S3 buckets and lazily load them into the in-memory storage, so that they can be read via IMAP clients.
