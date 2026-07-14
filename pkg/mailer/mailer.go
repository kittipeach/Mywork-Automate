// Package mailer builds and sends the emails behind the delivery.email node
// (spec 08 E8-S2, spec 05 §4.2). It has two concerns kept deliberately apart:
//
//   - BuildMIME / Message — pure, IO-free construction of a valid RFC 5322 +
//     MIME multipart/mixed message (a text body plus one optional base64
//     attachment). This is the unit-tested core (≥95% gate).
//   - Sender — the transport seam. The SMTP implementation (subpackage
//     mailer/smtp) is a thin net/smtp adapter, excluded from the unit gate.
//
// The executor renders recipients/subject/body via the expression engine, then
// hands a fully-populated Message to a Sender; the Sender never inspects the
// message beyond serialising it with BuildMIME.
package mailer

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// Message is a single email to build/send. Attachment is included only when
// AttachmentName is non-empty; the file bytes are base64-encoded in the MIME
// part. To carries one or more recipient addresses.
type Message struct {
	From           string
	To             []string
	Subject        string
	Body           string // plain-text body
	AttachmentName string // filename; empty => no attachment part
	Attachment     []byte // raw file bytes (base64-encoded into the part)
}

// Sender delivers a Message over some transport. The SMTP adapter lives in the
// mailer/smtp subpackage; tests inject a fake.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

const crlf = "\r\n"

// BuildMIME serialises m into a complete RFC 5322 message with a
// multipart/mixed body: part 1 is the plain-text body, part 2 (present only
// when AttachmentName is set) is the base64-encoded attachment. Line endings
// are CRLF as required by SMTP. It never fails: all inputs are strings/bytes
// spliced into a fixed structure.
func BuildMIME(m Message) []byte {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Envelope headers precede the multipart body. The Content-Type carries the
	// boundary chosen by the multipart writer.
	writeHeader(&buf, "From", m.From)
	writeHeader(&buf, "To", strings.Join(m.To, ", "))
	writeHeader(&buf, "Subject", m.Subject)
	writeHeader(&buf, "MIME-Version", "1.0")
	writeHeader(&buf, "Content-Type", "multipart/mixed; boundary="+w.Boundary())
	buf.WriteString(crlf)

	// Part 1: plain-text body.
	bodyHeader := textproto.MIMEHeader{
		"Content-Type":              {"text/plain; charset=utf-8"},
		"Content-Transfer-Encoding": {"7bit"},
	}
	bodyPart, _ := w.CreatePart(bodyHeader)
	_, _ = bodyPart.Write([]byte(m.Body))

	// Part 2: optional attachment (base64).
	if m.AttachmentName != "" {
		attHeader := textproto.MIMEHeader{
			"Content-Type":              {"application/octet-stream"},
			"Content-Transfer-Encoding": {"base64"},
			"Content-Disposition": {
				mime.FormatMediaType("attachment", map[string]string{"filename": m.AttachmentName}),
			},
		}
		attPart, _ := w.CreatePart(attHeader)
		_, _ = attPart.Write(encodeBase64Lines(m.Attachment))
	}

	// Close writes the trailing boundary; its only error is a double-close, which
	// cannot happen here.
	_ = w.Close()
	return buf.Bytes()
}

// writeHeader appends "Key: value" + CRLF.
func writeHeader(buf *bytes.Buffer, key, value string) {
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString(crlf)
}

// encodeBase64Lines base64-encodes data and wraps it at 76 columns per RFC 2045,
// with CRLF line endings.
func encodeBase64Lines(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	const lineLen = 76
	var out bytes.Buffer
	for i := 0; i < len(encoded); i += lineLen {
		end := i + lineLen
		if end > len(encoded) {
			end = len(encoded)
		}
		out.WriteString(encoded[i:end])
		out.WriteString(crlf)
	}
	return out.Bytes()
}

// FormatAddrList joins recipient addresses for logging/diagnostics.
func FormatAddrList(addrs []string) string {
	return fmt.Sprintf("[%s]", strings.Join(addrs, ", "))
}
