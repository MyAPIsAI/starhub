package config

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	_ "github.com/glebarez/go-sqlite"
)

const localEnvPath = ".env"
const cryptoSecretVariable = "STARHUB_CRYPTO_SECRET"

// LoadLocalEnvironment reads the working directory, never the go run executable
// directory. Existing nonempty process variables take precedence. No expansion
// or shell evaluation is performed on file contents.
func LoadLocalEnvironment() error {
	_, values, err := readLocalEnvironment()
	if err != nil {
		return err
	}
	for key, value := range values {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return errors.New("could not load .env into the process environment")
			}
		}
	}
	return nil
}

// EnsurePersistentCryptoSecret runs before common's process-wide key is used.
// Operator commands pass allowCreate=false so verification never invents keys.
func EnsurePersistentCryptoSecret(database DatabaseConfig, allowCreate bool) (bool, error) {
	if secret := os.Getenv(cryptoSecretVariable); secret != "" {
		if !validLocalCryptoSecret(secret) {
			return false, errors.New("STARHUB_CRYPTO_SECRET must not be blank or contain control characters")
		}
		return false, nil
	}
	mode := strings.TrimSpace(os.Getenv("STARHUB_ENV"))
	if mode != "" && mode != "development" {
		return false, errors.New("configure a persistent STARHUB_CRYPTO_SECRET; automatic generation is only available in development")
	}
	if !allowCreate {
		return false, errors.New("STARHUB_CRYPTO_SECRET is missing; restore the original environment or .env before running operator commands")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		lock, err := os.OpenFile(".env.lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			if err := lock.Close(); err != nil {
				_ = os.Remove(".env.lock")
				return false, errors.New("could not close the .env initialization lock")
			}
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return false, errors.New("could not lock .env for initialization; check directory permissions")
		}
		if time.Now().After(deadline) {
			return false, errors.New(".env initialization is locked; check for another startup process or a stale .env.lock")
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer os.Remove(".env.lock")

	// Another process may have generated the secret after our initial file read.
	content, values, err := readLocalEnvironment()
	if err != nil {
		return false, err
	}
	if secret := values[cryptoSecretVariable]; secret != "" {
		return false, os.Setenv(cryptoSecretVariable, secret)
	}
	if err := checkSecretInitializationDatabase(database); err != nil {
		return false, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return false, errors.New("could not generate a persistent crypto secret")
	}
	secret := hex.EncodeToString(key)
	newline := "\n"
	if bytes.Contains(content, []byte("\r\n")) {
		newline = "\r\n"
	}
	next := append([]byte(nil), content...)
	if len(next) > 0 && next[len(next)-1] != '\n' {
		next = append(next, newline...)
	}
	next = append(next, cryptoSecretVariable+"="+secret+newline...)
	if len(next) > 1<<20 {
		return false, errors.New(".env has no space for a crypto secret within its 1 MiB limit")
	}
	temporary, err := os.CreateTemp(".", ".env-init-*")
	if err != nil {
		return false, errors.New("could not create a private .env file; check directory permissions")
	}
	defer os.Remove(temporary.Name())
	if err := protectSecretFile(temporary.Name()); err != nil {
		_ = temporary.Close()
		return false, errors.New("could not restrict access to the generated .env file")
	}
	_, writeErr := temporary.Write(next)
	if writeErr == nil {
		writeErr = temporary.Sync()
	}
	closeErr := temporary.Close()
	if writeErr != nil || closeErr != nil {
		return false, errors.New("could not persist .env; no generated crypto secret was activated")
	}
	latest, _, err := readLocalEnvironment()
	if err != nil || !bytes.Equal(latest, content) {
		return false, errors.New(".env changed during initialization; retry without overwriting it")
	}
	if err := os.Rename(temporary.Name(), localEnvPath); err != nil {
		return false, errors.New("could not replace .env; no generated crypto secret was activated")
	}
	if err := os.Setenv(cryptoSecretVariable, secret); err != nil {
		return false, errors.New(".env was saved but its crypto secret could not be activated")
	}
	return true, nil
}

func readLocalEnvironment() ([]byte, map[string]string, error) {
	values := make(map[string]string)
	info, err := os.Lstat(localEnvPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, values, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, nil, errors.New(".env must be a readable regular file no larger than 1 MiB")
	}
	file, err := os.Open(localEnvPath)
	if err != nil {
		return nil, nil, errors.New("could not read .env")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(content) > 1<<20 {
		return nil, nil, errors.New("could not read a bounded .env file")
	}
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(content, []byte{0xef, 0xbb, 0xbf})))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	lineNumber := 0
	seen := make(map[string]bool)
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, raw, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if strings.HasPrefix(strings.ToUpper(key), "STARHUB_") {
			key = strings.ToUpper(key)
		}
		invalid := !found || key == ""
		for i, char := range key {
			if !(char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || i > 0 && char >= '0' && char <= '9') {
				invalid = true
			}
		}
		if seen[strings.ToUpper(key)] {
			invalid = true
		}
		value, err := parseLocalEnvironmentValue(raw)
		if invalid || err != nil || strings.IndexByte(value, 0) >= 0 {
			return nil, nil, fmt.Errorf("invalid or duplicate .env assignment at line %d; file contents are not displayed", lineNumber)
		}
		if key == cryptoSecretVariable && !validLocalCryptoSecret(value) {
			return nil, nil, errors.New(".env has an empty or invalid STARHUB_CRYPTO_SECRET; restore its original value rather than replacing it")
		}
		values[key] = value
		seen[strings.ToUpper(key)] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, errors.New("could not parse .env")
	}
	return content, values, nil
}

func parseLocalEnvironmentValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if value[0] == '#' {
		return "", nil
	}
	if value[0] != '\'' && value[0] != '"' {
		for i := 1; i < len(value); i++ {
			if value[i] == '#' && (value[i-1] == ' ' || value[i-1] == '\t') {
				value = strings.TrimSpace(value[:i])
				break
			}
		}
		return value, nil
	}
	quote := value[0]
	var result strings.Builder
	for i := 1; i < len(value); i++ {
		char := value[i]
		if char == quote {
			tail := strings.TrimSpace(value[i+1:])
			if tail != "" && !strings.HasPrefix(tail, "#") {
				return "", errors.New("unexpected text after quoted value")
			}
			return result.String(), nil
		}
		if char == '\\' && quote == '"' && i+1 < len(value) {
			switch value[i+1] {
			case '\\', '"':
				i++
				char = value[i]
			case 'n':
				i++
				char = '\n'
			case 'r':
				i++
				char = '\r'
			}
		}
		result.WriteByte(char)
	}
	return "", errors.New("unterminated quoted value")
}

// Inspect only local files through a read-only connection. Unknown drivers,
// URI DSNs and query failures cannot prove that generating a new key is safe.
func checkSecretInitializationDatabase(database DatabaseConfig) error {
	if database.Driver != "sqlite" || database.DSN == "" || database.DSN == ":memory:" || strings.HasPrefix(database.DSN, "file:") || strings.Contains(database.DSN, "?") {
		return errors.New("automatic crypto secret initialization requires a local SQLite file; configure STARHUB_CRYPTO_SECRET explicitly for this database")
	}
	info, err := os.Stat(database.DSN)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("could not safely inspect the database before generating a crypto secret")
	}
	absolute, err := filepath.Abs(database.DSN)
	if err != nil {
		return errors.New("could not resolve the database for crypto secret initialization")
	}
	path := filepath.ToSlash(absolute)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	uri := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return errors.New("could not open the database read-only for crypto secret initialization")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, table := range []string{"channels", "tokens", "sms_configurations"} {
		var exists int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&exists); err != nil {
			return errors.New("could not inspect credential storage; restore the original STARHUB_CRYPTO_SECRET before starting")
		}
		if exists == 0 {
			continue
		}
		var encrypted bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM "`+table+`" WHERE substr("key", 1, 4) = 'enc:')`).Scan(&encrypted); err != nil {
			return errors.New("could not inspect credential storage; restore the original STARHUB_CRYPTO_SECRET before starting")
		}
		if !encrypted && table == "tokens" {
			var hasHashColumn int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('tokens') WHERE name = 'key_hash'`).Scan(&hasHashColumn); err != nil {
				return errors.New("could not inspect token lookup storage")
			}
			if hasHashColumn > 0 {
				if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tokens WHERE key_hash IS NOT NULL AND key_hash <> '')`).Scan(&encrypted); err != nil {
					return errors.New("could not inspect token lookup storage")
				}
			}
		}
		if encrypted {
			return errors.New("database already contains encrypted credentials or token lookup hashes; restore the original STARHUB_CRYPTO_SECRET in the environment or .env; no new secret was generated")
		}
	}
	return nil
}

// Reject control characters in the persisted crypto secret without exposing it.
func validLocalCryptoSecret(value string) bool {
	return strings.TrimSpace(value) != "" && !strings.ContainsFunc(value, unicode.IsControl)
}
