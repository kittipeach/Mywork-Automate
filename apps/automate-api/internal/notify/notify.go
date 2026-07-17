// Package notify sends run-failure alerts (E5-S6: FR-RUN-005). When a flow run
// finishes "failed", the control-plane calls RunFailed with the flow's
// configured failure recipients (flowspec Settings.Notification.FailureEmails);
// the mailer-backed Notifier turns that into a plain-text email via a
// mailer.Sender (the SMTP adapter in production, a fake in tests).
//
// The message-building is a pure helper (buildFailureEmail) so the subject/body/
// recipient contract is unit-tested without any transport. Delivery is a
// best-effort side effect: the control-plane treats a RunFailed error as
// non-fatal (logged WARN), so a bad SMTP endpoint never breaks a run's recording.
package notify

import (
	"context"
	"fmt"

	"github.com/mywork/automate/pkg/mailer"
)

// FailureNotice is the input to RunFailed: which flow failed, the execution id
// that failed, the terminal error message, and who to notify. Recipients comes
// from the flow's Settings.Notification.FailureEmails; an empty list means the
// flow opted out of failure email and RunFailed is a no-op.
type FailureNotice struct {
	FlowName    string
	ExecutionID string
	Error       string
	Recipients  []string
}

// Notifier delivers run-failure alerts. RunFailed is best-effort from the
// caller's point of view (a returned error is logged, not fatal), but it returns
// the transport error so callers can log it and tests can assert on it.
type Notifier interface {
	RunFailed(ctx context.Context, in FailureNotice) error
}

// mailNotifier is the mailer-backed Notifier: it builds a plain-text failure
// email and hands it to a Sender.
type mailNotifier struct {
	sender mailer.Sender
}

// New builds a mailer-backed Notifier over the given Sender (the SMTP adapter in
// production). RunFailed is a no-op when a notice carries no recipients.
func New(sender mailer.Sender) Notifier {
	return &mailNotifier{sender: sender}
}

// RunFailed emails the notice's recipients about the failed run. With no
// recipients it does nothing (the flow opted out). A transport failure is
// wrapped and returned; the caller decides how loud that is.
func (n *mailNotifier) RunFailed(ctx context.Context, in FailureNotice) error {
	if len(in.Recipients) == 0 {
		return nil
	}
	msg := buildFailureEmail(in)
	if err := n.sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("notify: send failure email for %s: %w", in.ExecutionID, err)
	}
	return nil
}

// buildFailureEmail is the pure core: it maps a FailureNotice into a plain-text
// mailer.Message (no attachment). The From is left empty so the Sender fills in
// its configured envelope sender. Kept IO-free so the subject/body/recipient
// contract is unit-tested directly.
func buildFailureEmail(in FailureNotice) mailer.Message {
	subject := fmt.Sprintf("[MyWork Automate] Flow %q run failed", in.FlowName)
	body := fmt.Sprintf(
		"A flow run failed.\n\n"+
			"Flow:         %s\n"+
			"Execution ID: %s\n"+
			"Error:        %s\n",
		in.FlowName, in.ExecutionID, in.Error,
	)
	return mailer.Message{
		To:      in.Recipients,
		Subject: subject,
		Body:    body,
	}
}

// Noop is the Notifier used when SMTP is not configured: it accepts every notice
// and delivers nothing. The composition root wires it in place of the
// mailer-backed Notifier when no Sender is available.
type Noop struct{}

// RunFailed does nothing and never errors.
func (Noop) RunFailed(context.Context, FailureNotice) error { return nil }

// compile-time assertions that both implementations satisfy Notifier.
var (
	_ Notifier = (*mailNotifier)(nil)
	_ Notifier = Noop{}
)
