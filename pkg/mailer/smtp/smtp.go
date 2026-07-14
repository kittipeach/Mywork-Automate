// Package smtp is the thin net/smtp adapter implementing mailer.Sender
// (spec 08 E8-S2). It serialises a mailer.Message with mailer.BuildMIME and
// hands the bytes to net/smtp.SendMail against a configured host:port.
//
// This is intentionally a thin network adapter: the message-construction logic
// (headers, MIME, base64) lives and is unit-tested in the parent mailer package,
// so this file is excluded from the unit-coverage gate — it can only be
// exercised against a live SMTP endpoint (mailhog) in the E8-S2 integration test.
package smtp

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/mywork/automate/pkg/mailer"
)

// Sender sends mail over SMTP. Auth is optional (nil for an open relay such as
// the dev mailhog); From is the envelope sender.
type Sender struct {
	Addr string    // "host:port"
	From string    // envelope MAIL FROM
	Auth smtp.Auth // optional
	// send is the net/smtp.SendMail seam (overridable in tests).
	send func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// New builds an SMTP Sender for addr ("host:port") with envelope sender from.
func New(addr, from string) *Sender {
	return &Sender{Addr: addr, From: from, send: smtp.SendMail}
}

// WithAuth attaches SMTP AUTH credentials (PLAIN).
func (s *Sender) WithAuth(username, password, host string) *Sender {
	s.Auth = smtp.PlainAuth("", username, password, host)
	return s
}

// Send serialises m (using s.From when m.From is empty) and delivers it via
// net/smtp. The context is accepted for interface parity; net/smtp does not
// support cancellation, so it bounds nothing here.
func (s *Sender) Send(_ context.Context, m mailer.Message) error {
	if m.From == "" {
		m.From = s.From
	}
	raw := mailer.BuildMIME(m)
	if err := s.send(s.Addr, s.Auth, m.From, m.To, raw); err != nil {
		return fmt.Errorf("mailer/smtp: send to %s via %s: %w", mailer.FormatAddrList(m.To), s.Addr, err)
	}
	return nil
}

// compile-time assertion that Sender satisfies mailer.Sender.
var _ mailer.Sender = (*Sender)(nil)
