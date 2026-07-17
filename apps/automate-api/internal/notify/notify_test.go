package notify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mywork/automate/pkg/mailer"
)

// fakeSender records the last Message it was asked to send and can be primed to
// return an error, so the mailer-backed Notifier can be tested without SMTP.
type fakeSender struct {
	sent []mailer.Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, m mailer.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

// --- pure helper: buildFailureEmail ---

func TestBuildFailureEmail_SubjectBodyRecipients(t *testing.T) {
	in := FailureNotice{
		FlowName:    "Payroll → Bank MFT",
		ExecutionID: "exe_abc123",
		Error:       "connection timeout to hr-db",
		Recipients:  []string{"ops@mywork.local", "oncall@mywork.local"},
	}
	msg := buildFailureEmail(in)

	wantSubject := `[MyWork Automate] Flow "Payroll → Bank MFT" run failed`
	if msg.Subject != wantSubject {
		t.Errorf("subject = %q, want %q", msg.Subject, wantSubject)
	}
	if len(msg.To) != 2 || msg.To[0] != "ops@mywork.local" || msg.To[1] != "oncall@mywork.local" {
		t.Errorf("recipients = %v, want the notice recipients", msg.To)
	}
	// Body must name the flow, the execution id and the error so the on-call
	// engineer can act without opening the console.
	for _, want := range []string{"Payroll → Bank MFT", "exe_abc123", "connection timeout to hr-db"} {
		if !strings.Contains(msg.Body, want) {
			t.Errorf("body missing %q:\n%s", want, msg.Body)
		}
	}
	// A plain-text failure email carries no attachment.
	if msg.AttachmentName != "" || msg.Attachment != nil {
		t.Errorf("failure email must be plain text with no attachment, got name=%q", msg.AttachmentName)
	}
}

// --- mailer-backed Notifier ---

func TestNotifier_RunFailed_SendsEmail(t *testing.T) {
	fs := &fakeSender{}
	n := New(fs)

	in := FailureNotice{
		FlowName:    "Leave Report",
		ExecutionID: "exe_1002",
		Error:       "boom",
		Recipients:  []string{"hr@mywork.local"},
	}
	if err := n.RunFailed(context.Background(), in); err != nil {
		t.Fatalf("RunFailed returned error: %v", err)
	}
	if len(fs.sent) != 1 {
		t.Fatalf("expected exactly one email sent, got %d", len(fs.sent))
	}
	got := fs.sent[0]
	if got.Subject != `[MyWork Automate] Flow "Leave Report" run failed` {
		t.Errorf("subject = %q", got.Subject)
	}
	if len(got.To) != 1 || got.To[0] != "hr@mywork.local" {
		t.Errorf("recipients = %v", got.To)
	}
}

func TestNotifier_RunFailed_NoRecipients_NoSend(t *testing.T) {
	tests := []struct {
		name       string
		recipients []string
	}{
		{"nil recipients", nil},
		{"empty recipients", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeSender{}
			n := New(fs)
			err := n.RunFailed(context.Background(), FailureNotice{
				FlowName:    "X",
				ExecutionID: "exe_x",
				Error:       "e",
				Recipients:  tt.recipients,
			})
			if err != nil {
				t.Fatalf("RunFailed returned error: %v", err)
			}
			if len(fs.sent) != 0 {
				t.Fatalf("expected no email sent for empty recipients, got %d", len(fs.sent))
			}
		})
	}
}

func TestNotifier_RunFailed_SendError_Propagates(t *testing.T) {
	fs := &fakeSender{err: errors.New("smtp down")}
	n := New(fs)
	err := n.RunFailed(context.Background(), FailureNotice{
		FlowName:    "X",
		ExecutionID: "exe_x",
		Error:       "e",
		Recipients:  []string{"a@b.c"},
	})
	if err == nil {
		t.Fatal("expected the sender error to propagate")
	}
	if !strings.Contains(err.Error(), "smtp down") {
		t.Errorf("error = %v, want it to wrap the sender error", err)
	}
}

// --- Noop ---

func TestNoop_RunFailed_NeverErrors(t *testing.T) {
	var n Notifier = Noop{}
	if err := n.RunFailed(context.Background(), FailureNotice{
		FlowName:    "X",
		ExecutionID: "exe_x",
		Error:       "e",
		Recipients:  []string{"a@b.c"},
	}); err != nil {
		t.Fatalf("Noop.RunFailed must never error, got %v", err)
	}
}
