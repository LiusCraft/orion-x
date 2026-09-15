package main

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestRunHashPasswordFromArgument(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"hash-password", "correct horse battery staple"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	hash := strings.TrimSpace(out.String())
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct horse battery staple")); err != nil {
		t.Fatalf("generated hash does not match password: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestRunHashPasswordFromStdin(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"hash-password"}, strings.NewReader("stdin secret\n"), &out, &errOut); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	hash := strings.TrimSpace(out.String())
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("stdin secret")); err != nil {
		t.Fatalf("generated hash does not match password: %v", err)
	}
	if errOut.String() != "Password: " {
		t.Fatalf("unexpected prompt: %q", errOut.String())
	}
}

func TestRunHashPasswordRejectsConflictingInputs(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"hash-password", "-password", "one", "two"}, strings.NewReader(""), &out, &errOut); err == nil {
		t.Fatal("expected conflicting input error")
	}
}
