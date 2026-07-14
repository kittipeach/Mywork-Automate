package smtp

import (
	"context"
	"errors"
	"testing"

	netsmtp "net/smtp"

	"github.com/stretchr/testify/require"

	"github.com/mywork/automate/pkg/mailer"
)

func TestSend_UsesSeamAndDefaultFrom(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte
	s := New("localhost:1025", "noreply@ttb.example")
	s.send = func(addr string, _ netsmtp.Auth, from string, to []string, msg []byte) error {
		gotAddr, gotFrom, gotTo, gotMsg = addr, from, to, msg
		return nil
	}

	err := s.Send(context.Background(), mailer.Message{
		To:      []string{"d@x"},
		Subject: "Hi",
		Body:    "b",
	})
	require.NoError(t, err)
	require.Equal(t, "localhost:1025", gotAddr)
	require.Equal(t, "noreply@ttb.example", gotFrom) // default From applied
	require.Equal(t, []string{"d@x"}, gotTo)
	require.Contains(t, string(gotMsg), "Subject: Hi")
}

func TestSend_PropagatesError(t *testing.T) {
	s := New("localhost:1025", "s@x")
	s.send = func(string, netsmtp.Auth, string, []string, []byte) error {
		return errors.New("dial refused")
	}
	err := s.Send(context.Background(), mailer.Message{To: []string{"d@x"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "dial refused")
}

func TestWithAuth_SetsAuth(t *testing.T) {
	s := New("localhost:1025", "s@x").WithAuth("user", "pass", "localhost")
	require.NotNil(t, s.Auth)
}
