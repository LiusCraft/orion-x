// Command tools contains small administrative utilities for Orion-X.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const hashPasswordCommand = "hash-password"

var errUsage = errors.New("invalid command usage")

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "tools:", err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 0 {
		printUsage(errOut)
		return errUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		printUsage(out)
		return nil
	case hashPasswordCommand:
		return runHashPassword(args[1:], in, out, errOut)
	default:
		printUsage(errOut)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runHashPassword(args []string, in io.Reader, out, errOut io.Writer) error {
	flags := flag.NewFlagSet(hashPasswordCommand, flag.ContinueOnError)
	flags.SetOutput(errOut)
	passwordFlag := flags.String("password", "", "plaintext password (prefer stdin to avoid shell history)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *passwordFlag != "" && len(flags.Args()) > 0 {
		return errors.New("use either -password or one positional password, not both")
	}

	password := *passwordFlag
	if len(flags.Args()) > 1 {
		return errors.New("hash-password accepts at most one positional password")
	}
	if len(flags.Args()) == 1 {
		password = flags.Args()[0]
	}
	if password == "" {
		if _, err := fmt.Fprint(errOut, "Password: "); err != nil {
			return fmt.Errorf("write prompt: %w", err)
		}
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read password: %w", err)
		}
		password = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	}
	if password == "" {
		return errors.New("password must not be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = fmt.Fprintln(out, string(hash))
	return err
}

func printUsage(out io.Writer) {
	_, _ = fmt.Fprintln(out, `Usage:
  tools hash-password [password]
  tools hash-password -password password
  echo password | tools hash-password

hash-password generates a bcrypt hash compatible with users.password_hash.`)
}
