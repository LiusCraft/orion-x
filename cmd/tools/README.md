# Tools

Generate a bcrypt password hash compatible with the manager's `users.password_hash` field:

```bash
go run ./cmd/tools hash-password 'your password'
```

To keep the password out of shell history, pipe it through standard input:

```bash
printf '%s\n' 'your password' | go run ./cmd/tools hash-password
```

The command prints only the bcrypt hash. It does not modify the database.
