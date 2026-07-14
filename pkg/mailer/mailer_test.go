package mailer

import (
	"bytes"
	"encoding/base64"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildMIME_HeadersAndStructure(t *testing.T) {
	m := Message{
		From:    "noreply@ttb.example",
		To:      []string{"a@ttb.example", "b@ttb.example"},
		Subject: "Payroll 2026-07",
		Body:    "See attached.",
	}
	raw := BuildMIME(m)

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, "noreply@ttb.example", msg.Header.Get("From"))
	require.Equal(t, "a@ttb.example, b@ttb.example", msg.Header.Get("To"))
	require.Equal(t, "Payroll 2026-07", msg.Header.Get("Subject"))
	require.Equal(t, "1.0", msg.Header.Get("MIME-Version"))

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/mixed", mediaType)
	require.NotEmpty(t, params["boundary"])
}

func TestBuildMIME_BodyPartOnly_NoAttachment(t *testing.T) {
	m := Message{
		From:    "s@x",
		To:      []string{"d@x"},
		Subject: "Hi",
		Body:    "hello world",
	}
	raw := BuildMIME(m)
	parts := readParts(t, raw)
	require.Len(t, parts, 1)
	require.Equal(t, "hello world", parts[0].body)
	require.Contains(t, parts[0].contentType, "text/plain")
}

func TestBuildMIME_WithAttachment(t *testing.T) {
	payload := []byte("id,name\n1,alice\n")
	m := Message{
		From:           "s@x",
		To:             []string{"d@x"},
		Subject:        "Report",
		Body:           "body text",
		AttachmentName: "report.csv",
		Attachment:     payload,
	}
	raw := BuildMIME(m)
	parts := readParts(t, raw)
	require.Len(t, parts, 2)

	// Part 0: body.
	require.Equal(t, "body text", parts[0].body)
	require.Contains(t, parts[0].contentType, "text/plain")

	// Part 1: attachment, base64-encoded, with a filename.
	require.Contains(t, parts[1].contentType, "application/octet-stream")
	require.Equal(t, "base64", parts[1].encoding)
	require.Contains(t, parts[1].disposition, "attachment")
	require.Contains(t, parts[1].disposition, "report.csv")

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1].body))
	require.NoError(t, err)
	require.Equal(t, payload, decoded)
}

func TestBuildMIME_EmptyAttachmentNameSkipsAttachment(t *testing.T) {
	// Bytes present but no name -> no attachment part (name is the trigger).
	m := Message{
		From:       "s@x",
		To:         []string{"d@x"},
		Subject:    "x",
		Body:       "b",
		Attachment: []byte("data"),
	}
	raw := BuildMIME(m)
	parts := readParts(t, raw)
	require.Len(t, parts, 1)
}

func TestBuildMIME_CRLFLineEndings(t *testing.T) {
	m := Message{From: "s@x", To: []string{"d@x"}, Subject: "x", Body: "b"}
	raw := BuildMIME(m)
	// SMTP requires CRLF; headers must be CRLF-terminated.
	require.Contains(t, string(raw), "\r\n")
	require.NotContains(t, strings.ReplaceAll(string(raw), "\r\n", ""), "\n")
}

func TestFormatAddrList(t *testing.T) {
	require.Equal(t, "[a@x, b@x]", FormatAddrList([]string{"a@x", "b@x"}))
	require.Equal(t, "[]", FormatAddrList(nil))
}

// part is a flattened MIME part for assertions.
type part struct {
	contentType string
	encoding    string
	disposition string
	body        string
}

// readParts parses a multipart/mixed message and returns its parts.
func readParts(t *testing.T, raw []byte) []part {
	t.Helper()
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	require.NoError(t, err)
	mr := multipart.NewReader(msg.Body, params["boundary"])

	var parts []part
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		var b bytes.Buffer
		_, _ = b.ReadFrom(p)
		parts = append(parts, part{
			contentType: p.Header.Get("Content-Type"),
			encoding:    p.Header.Get("Content-Transfer-Encoding"),
			disposition: p.Header.Get("Content-Disposition"),
			body:        b.String(),
		})
	}
	return parts
}
