package store

import (
	"errors"
	"testing"
)

func TestNotFound(t *testing.T) {
	err := NewNotFound("flow xyz not found")
	if err.Error() != "flow xyz not found" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !IsNotFound(err) {
		t.Error("IsNotFound should be true for a NewNotFound error")
	}
	if IsNotFound(errors.New("other")) {
		t.Error("IsNotFound should be false for a plain error")
	}
	if IsNotFound(nil) {
		t.Error("IsNotFound(nil) should be false")
	}
}

func TestConnectionDSN(t *testing.T) {
	tests := []struct {
		name string
		c    Connection
		pass string
		want string
	}{
		{
			name: "full postgres",
			c:    Connection{Type: "postgres", Host: "hr-db", Port: 5432, Database: "hr", Username: "reader", SSLMode: "require"},
			pass: "s3cr3t",
			want: "postgres://reader:s3cr3t@hr-db:5432/hr?sslmode=require",
		},
		{
			name: "defaults port 5432 and sslmode disable",
			c:    Connection{Type: "postgres", Host: "db", Database: "app", Username: "u"},
			pass: "p",
			want: "postgres://u:p@db:5432/app?sslmode=disable",
		},
		{
			name: "escapes special chars in user/password",
			c:    Connection{Type: "postgres", Host: "db", Database: "app", Username: "a@b", Port: 6000},
			pass: "p@ss/word",
			want: "postgres://a%40b:p%40ss%2Fword@db:6000/app?sslmode=disable",
		},
		{name: "non-postgres type => empty", c: Connection{Type: "sftp", Host: "h", Database: "d", Username: "u"}, pass: "p", want: ""},
		{name: "missing database => empty", c: Connection{Type: "postgres", Host: "h", Username: "u"}, pass: "p", want: ""},
		{name: "missing username => empty", c: Connection{Type: "postgres", Host: "h", Database: "d"}, pass: "p", want: ""},
		{name: "missing host => empty", c: Connection{Type: "postgres", Database: "d", Username: "u"}, pass: "p", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.DSN(tc.pass); got != tc.want {
				t.Errorf("DSN() = %q, want %q", got, tc.want)
			}
		})
	}
}
