package terminal

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// SanitizedResult contains the sanitized command details and privacy flags.
type SanitizedResult struct {
	Executable       string
	Args             []string
	SanitizedCommand string
	IsRedacted       bool
	IsOmitted        bool
	OmissionReason   string
}

var (
	// Tools whose execution details (arguments) are inherently sensitive and must be omitted.
	sensitiveTools = map[string]bool{
		"pass":      true,
		"op":        true,
		"bw":        true,
		"vault":     true,
		"security":  true,
		"gpg":       true,
		"gpg2":      true,
		"ssh-add":   true,
		"kinit":     true,
		"keychain":  true,
		"aws-vault": true,
	}

	// Commands where short '-p' represents a password rather than a port.
	passwordShortFlagTools = map[string]bool{
		"mysql":     true,
		"mysqldump": true,
		"mariadb":   true,
		"redis-cli": true,
	}

	// Regex for credentials inside URLs (e.g., https://user:secret@host/path)
	urlCredentialsRegex = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^:/@\s]+):([^@\s/]+)@`)

	// Regex for well-known token formats
	githubTokenRegex = regexp.MustCompile(`\bgh[pousr]_[a-zA-Z0-9]{20,}\b`)
	awsKeyRegex      = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	slackTokenRegex  = regexp.MustCompile(`\bxox[baprs]-[0-9a-zA-Z]{10,}\b`)
	openAITokenRegex = regexp.MustCompile(`\bsk-[a-zA-Z0-9]{20,}\b`)
	bearerRegex      = regexp.MustCompile(`(?i)\bBearer\s+[a-zA-Z0-9_\-\.~+/]+=*`)
	privateKeyRegex  = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----.*`)
)

// Sanitize analyzes and sanitizes command arguments before storage.
// It redacts sensitive flags, secret assignments, bearer tokens, API keys,
// and omits details for credential managers and authentication tools.
func Sanitize(rawArgs []string, isShell bool) SanitizedResult {
	if len(rawArgs) == 0 {
		return SanitizedResult{
			Executable:       "",
			Args:             nil,
			SanitizedCommand: "",
		}
	}

	exe := rawArgs[0]
	baseExe := strings.ToLower(filepath.Base(exe))

	// 1. Check for sensitive tools whose arguments must be omitted
	if isSensitiveTool(baseExe, rawArgs[1:]) {
		reason := "sensitive authentication or credential tool"
		return SanitizedResult{
			Executable:       baseExe,
			Args:             nil,
			SanitizedCommand: fmt.Sprintf("%s [DETAILS OMITTED: %s]", baseExe, reason),
			IsRedacted:       true,
			IsOmitted:        true,
			OmissionReason:   reason,
		}
	}

	// 2. If shell mode is explicitly requested, sanitize as a shell command line
	if isShell {
		shellCmd := strings.Join(rawArgs, " ")
		sanitizedShell, redacted := sanitizeString(shellCmd)
		return SanitizedResult{
			Executable:       "sh",
			Args:             []string{"-c", sanitizedShell},
			SanitizedCommand: fmt.Sprintf("sh -c %s", quoteArg(sanitizedShell)),
			IsRedacted:       redacted,
			IsOmitted:        false,
		}
	}

	// 3. Direct mode: inspect executable and arguments
	sanitizedArgs := make([]string, 0, len(rawArgs)-1)
	isRedacted := false

	for i := 1; i < len(rawArgs); i++ {
		arg := rawArgs[i]

		// Check for sensitive flag with '=VALUE' (e.g., --password=secret, -token=secret)
		if flagName, _, ok := splitFlagValue(arg); ok && isSensitiveFlagName(flagName) {
			prefix := arg[:strings.IndexByte(arg, '=')+1]
			sanitizedArgs = append(sanitizedArgs, prefix+"[REDACTED]")
			isRedacted = true
			continue
		}

		// Check for sensitive flag with separate next argument (e.g., --password secret, -token secret)
		if isFlag(arg) {
			flagName := trimFlagDashes(arg)
			if isSensitiveFlagName(flagName) && i+1 < len(rawArgs) {
				sanitizedArgs = append(sanitizedArgs, arg)
				sanitizedArgs = append(sanitizedArgs, "[REDACTED]")
				i++ // skip next arg
				isRedacted = true
				continue
			}
		}

		// Check for MySQL/MariaDB/Redis short password flag (e.g., -pSecret or -p Secret)
		if passwordShortFlagTools[baseExe] {
			if arg == "-p" || arg == "-a" {
				sanitizedArgs = append(sanitizedArgs, arg)
				if i+1 < len(rawArgs) {
					sanitizedArgs = append(sanitizedArgs, "[REDACTED]")
					i++
					isRedacted = true
					continue
				}
			} else if strings.HasPrefix(arg, "-p") && len(arg) > 2 && !strings.HasPrefix(arg, "-p=") {
				sanitizedArgs = append(sanitizedArgs, "-p[REDACTED]")
				isRedacted = true
				continue
			}
		}

		// Check for HTTP headers (e.g., -H "Authorization: Bearer secret", --header "X-Api-Key: secret")
		if (arg == "-H" || arg == "--header") && i+1 < len(rawArgs) {
			sanitizedArgs = append(sanitizedArgs, arg)
			nextArg := rawArgs[i+1]
			sanitizedHdr, hdrRedacted := sanitizeHeader(nextArg)
			sanitizedArgs = append(sanitizedArgs, sanitizedHdr)
			if hdrRedacted {
				isRedacted = true
			}
			i++
			continue
		}

		// Check for inline environment variable assignments (e.g., AWS_SECRET_ACCESS_KEY=secret, TOKEN=secret)
		if key, _, ok := isSensitiveEnvAssignment(arg); ok {
			sanitizedArgs = append(sanitizedArgs, key+"=[REDACTED]")
			isRedacted = true
			continue
		}

		// General secret patterns inside argument values
		sanitizedVal, valRedacted := sanitizeString(arg)
		sanitizedArgs = append(sanitizedArgs, sanitizedVal)
		if valRedacted {
			isRedacted = true
		}
	}

	sanitizedCmd := formatCommand(baseExe, sanitizedArgs)

	return SanitizedResult{
		Executable:       baseExe,
		Args:             sanitizedArgs,
		SanitizedCommand: sanitizedCmd,
		IsRedacted:       isRedacted,
		IsOmitted:        false,
	}
}

// isSensitiveTool checks if the executable or command pattern is a credential manager or auth tool.
func isSensitiveTool(baseExe string, args []string) bool {
	if sensitiveTools[baseExe] {
		return true
	}

	// Tool + subcommand combinations
	if len(args) > 0 {
		subcmd := strings.ToLower(args[0])
		switch baseExe {
		case "gh":
			if subcmd == "auth" {
				return true
			}
		case "docker", "podman":
			if subcmd == "login" {
				return true
			}
		case "npm", "yarn":
			if subcmd == "login" {
				return true
			}
		case "aws":
			if subcmd == "configure" {
				return true
			}
		}
	}

	return false
}

// splitFlagValue splits `--name=val` into name and val.
func splitFlagValue(arg string) (string, string, bool) {
	if !isFlag(arg) {
		return "", "", false
	}
	idx := strings.IndexByte(arg, '=')
	if idx <= 0 {
		return "", "", false
	}
	name := trimFlagDashes(arg[:idx])
	val := arg[idx+1:]
	return name, val, true
}

func isFlag(arg string) bool {
	return strings.HasPrefix(arg, "-") && len(arg) > 1
}

func trimFlagDashes(arg string) string {
	return strings.TrimLeft(arg, "-")
}

// isSensitiveFlagName returns true if a flag name matches sensitive keyword heuristics.
func isSensitiveFlagName(name string) bool {
	lower := strings.ToLower(name)
	sensitiveKeys := []string{
		"password", "passwd", "pass", "pwd",
		"token", "api-token", "apitoken", "access-token", "accesstoken",
		"auth-token", "authtoken", "bearer-token", "bearertoken",
		"personal-access-token", "pat", "session-token",
		"secret", "client-secret", "clientsecret", "app-secret", "appsecret",
		"api-key", "apikey", "secret-key", "secretkey", "private-key", "privatekey",
		"app-key", "appkey", "access-key", "accesskey", "client-key",
		"authorization", "auth-header", "auth",
		"credential", "credentials",
		"privkey", "private-cert",
	}

	for _, k := range sensitiveKeys {
		if lower == k || strings.HasSuffix(lower, "-"+k) || strings.HasSuffix(lower, "_"+k) {
			return true
		}
	}
	return false
}

// isSensitiveEnvAssignment checks if arg is an environment assignment `KEY=VALUE` with sensitive name.
func isSensitiveEnvAssignment(arg string) (string, string, bool) {
	idx := strings.IndexByte(arg, '=')
	if idx <= 0 {
		return "", "", false
	}
	key := arg[:idx]
	val := arg[idx+1:]

	// Validate key identifier characters
	for i, r := range key {
		if i == 0 && (r >= '0' && r <= '9') {
			return "", "", false
		}
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return "", "", false
		}
	}

	upper := strings.ToUpper(key)
	if strings.Contains(upper, "PASS") ||
		strings.Contains(upper, "TOKEN") ||
		strings.Contains(upper, "SECRET") ||
		strings.Contains(upper, "KEY") ||
		strings.Contains(upper, "AUTH") ||
		strings.Contains(upper, "CRED") {
		return key, val, true
	}
	return "", "", false
}

// sanitizeHeader checks HTTP header arguments for sensitive values like Authorization or API keys.
func sanitizeHeader(header string) (string, bool) {
	colonIdx := strings.IndexByte(header, ':')
	if colonIdx <= 0 {
		return sanitizeString(header)
	}

	name := strings.TrimSpace(header[:colonIdx])
	lowerName := strings.ToLower(name)

	if lowerName == "authorization" ||
		lowerName == "proxy-authorization" ||
		strings.Contains(lowerName, "token") ||
		strings.Contains(lowerName, "api-key") ||
		strings.Contains(lowerName, "apikey") ||
		strings.Contains(lowerName, "secret") {
		return fmt.Sprintf("%s: [REDACTED]", name), true
	}

	sanitizedVal, redacted := sanitizeString(header[colonIdx+1:])
	return fmt.Sprintf("%s:%s", name, sanitizedVal), redacted
}

// sanitizeString applies token regex and URL credential replacements to a string.
func sanitizeString(s string) (string, bool) {
	original := s
	result := s

	// 1. Redact URL embedded credentials (e.g., https://user:pass@host)
	result = urlCredentialsRegex.ReplaceAllString(result, "$1:[REDACTED]@")

	// 2. Redact GitHub personal tokens
	result = githubTokenRegex.ReplaceAllString(result, "ghp_[REDACTED]")

	// 3. Redact AWS access keys
	result = awsKeyRegex.ReplaceAllString(result, "AKIA[REDACTED]")

	// 4. Redact Slack tokens
	result = slackTokenRegex.ReplaceAllString(result, "xox[REDACTED]")

	// 5. Redact OpenAI tokens
	result = openAITokenRegex.ReplaceAllString(result, "sk-[REDACTED]")

	// 6. Redact Bearer tokens
	result = bearerRegex.ReplaceAllString(result, "Bearer [REDACTED]")

	// 7. Redact private key headers
	result = privateKeyRegex.ReplaceAllString(result, "[REDACTED_PRIVATE_KEY]")

	return result, result != original
}

// formatCommand builds a display command string with necessary quoting.
func formatCommand(exe string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, exe)
	for _, a := range args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	if s == "[REDACTED]" {
		return s
	}
	if strings.ContainsAny(s, " \t\n\r\"'$`\\*?~<>|&;()") {
		return strconv.Quote(s)
	}
	return s
}
