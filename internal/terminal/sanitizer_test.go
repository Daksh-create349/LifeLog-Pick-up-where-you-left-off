package terminal

import (
	"strings"
	"testing"
)

func TestSanitizeSensitiveTools(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
		omitted  bool
	}{
		{
			name:     "pass show",
			args:     []string{"pass", "show", "email/gmail"},
			expected: "pass [DETAILS OMITTED: sensitive authentication or credential tool]",
			omitted:  true,
		},
		{
			name:     "security find-generic-password",
			args:     []string{"security", "find-generic-password", "-s", "my_service", "-w"},
			expected: "security [DETAILS OMITTED: sensitive authentication or credential tool]",
			omitted:  true,
		},
		{
			name:     "op item get",
			args:     []string{"op", "item", "get", "db-password", "--fields", "password"},
			expected: "op [DETAILS OMITTED: sensitive authentication or credential tool]",
			omitted:  true,
		},
		{
			name:     "vault kv get",
			args:     []string{"vault", "kv", "get", "secret/data/app"},
			expected: "vault [DETAILS OMITTED: sensitive authentication or credential tool]",
			omitted:  true,
		},
		{
			name:     "gh auth login",
			args:     []string{"gh", "auth", "login", "--with-token"},
			expected: "gh [DETAILS OMITTED: sensitive authentication or credential tool]",
			omitted:  true,
		},
		{
			name:     "docker login",
			args:     []string{"docker", "login", "-u", "alice", "-p", "secret123"},
			expected: "docker [DETAILS OMITTED: sensitive authentication or credential tool]",
			omitted:  true,
		},
		{
			name:     "aws configure",
			args:     []string{"aws", "configure", "set", "aws_secret_access_key", "xxx"},
			expected: "aws [DETAILS OMITTED: sensitive authentication or credential tool]",
			omitted:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Sanitize(tt.args, false)
			if !res.IsOmitted {
				t.Fatalf("expected IsOmitted=true, got %v", res.IsOmitted)
			}
			if !res.IsRedacted {
				t.Fatalf("expected IsRedacted=true, got %v", res.IsRedacted)
			}
			if res.SanitizedCommand != tt.expected {
				t.Fatalf("expected SanitizedCommand %q, got %q", tt.expected, res.SanitizedCommand)
			}
			if len(res.Args) != 0 {
				t.Fatalf("expected Args to be empty for omitted tool, got %v", res.Args)
			}
		})
	}
}

func TestSanitizeFlagsAndAssignments(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantSubstr string
		isRedacted bool
	}{
		{
			name:       "flag with equal password",
			args:       []string{"curl", "--password=supersecret", "https://example.com"},
			wantSubstr: "--password=[REDACTED]",
			isRedacted: true,
		},
		{
			name:       "flag with space token",
			args:       []string{"deploy-cli", "--token", "secret-token-value", "--target", "prod"},
			wantSubstr: "--token [REDACTED]",
			isRedacted: true,
		},
		{
			name:       "flag with equal api-key",
			args:       []string{"mytool", "--api-key=xyz123456", "--verbose"},
			wantSubstr: "--api-key=[REDACTED]",
			isRedacted: true,
		},
		{
			name:       "flag with space secret-key",
			args:       []string{"mytool", "-secret-key", "mysecret"},
			wantSubstr: "-secret-key [REDACTED]",
			isRedacted: true,
		},
		{
			name:       "mysql short password flag attached",
			args:       []string{"mysql", "-u", "root", "-pMySecretPass", "mydb"},
			wantSubstr: "-p[REDACTED]",
			isRedacted: true,
		},
		{
			name:       "mysql short password flag separated",
			args:       []string{"mysql", "-u", "root", "-p", "MySecretPass", "mydb"},
			wantSubstr: "-p [REDACTED]",
			isRedacted: true,
		},
		{
			name:       "http authorization header",
			args:       []string{"curl", "-H", "Authorization: Bearer secret_token", "https://api.test"},
			wantSubstr: "-H \"Authorization: [REDACTED]\"",
			isRedacted: true,
		},
		{
			name:       "http api key header",
			args:       []string{"curl", "--header", "X-Api-Key: 12345", "https://api.test"},
			wantSubstr: "--header \"X-Api-Key: [REDACTED]\"",
			isRedacted: true,
		},
		{
			name:       "inline env assignment password",
			args:       []string{"env", "DB_PASSWORD=secret", "./run.sh"},
			wantSubstr: "DB_PASSWORD=[REDACTED]",
			isRedacted: true,
		},
		{
			name:       "inline env assignment token",
			args:       []string{"node", "SLACK_BOT_TOKEN=xoxb-123456789012", "app.js"},
			wantSubstr: "SLACK_BOT_TOKEN=[REDACTED]",
			isRedacted: true,
		},
		{
			name:       "url with credentials",
			args:       []string{"git", "clone", "https://user:password123@github.com/org/repo.git"},
			wantSubstr: "https://user:[REDACTED]@github.com/org/repo.git",
			isRedacted: true,
		},
		{
			name:       "github token in arbitrary argument",
			args:       []string{"echo", "ghp_123456789012345678901234567890"},
			wantSubstr: "ghp_[REDACTED]",
			isRedacted: true,
		},
		{
			name:       "aws key in arbitrary argument",
			args:       []string{"echo", "AKIAIOSFODNN7EXAMPLE"},
			wantSubstr: "AKIA[REDACTED]",
			isRedacted: true,
		},
		{
			name:       "innocent flag and argument",
			args:       []string{"git", "commit", "-m", "fix: regular commit message with spaces"},
			wantSubstr: "-m \"fix: regular commit message with spaces\"",
			isRedacted: false,
		},
		{
			name:       "innocent unicode argument",
			args:       []string{"echo", "Hello 世界 🚀"},
			wantSubstr: "\"Hello 世界 🚀\"",
			isRedacted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Sanitize(tt.args, false)
			if res.IsRedacted != tt.isRedacted {
				t.Fatalf("expected IsRedacted=%v, got %v for command: %s", tt.isRedacted, res.IsRedacted, res.SanitizedCommand)
			}
			if !strings.Contains(res.SanitizedCommand, tt.wantSubstr) {
				t.Fatalf("expected SanitizedCommand to contain %q, got: %s", tt.wantSubstr, res.SanitizedCommand)
			}
		})
	}
}

func TestSanitizeShellMode(t *testing.T) {
	cmdArgs := []string{"export", "GITHUB_TOKEN=ghp_123456789012345678901234567890", "&&", "./deploy"}
	res := Sanitize(cmdArgs, true)

	if !res.IsRedacted {
		t.Fatalf("expected shell mode with secret to be redacted")
	}
	if strings.Contains(res.SanitizedCommand, "ghp_123456789012345678901234567890") {
		t.Fatalf("secret leaked in shell command: %s", res.SanitizedCommand)
	}
	if !strings.Contains(res.SanitizedCommand, "ghp_[REDACTED]") {
		t.Fatalf("expected redacted token in shell command: %s", res.SanitizedCommand)
	}
}
