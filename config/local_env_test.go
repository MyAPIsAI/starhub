package config_test

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"starhub/common"
	"starhub/config"
	"starhub/internal/operator"
	"starhub/model"
)

func localEnvCommand(t *testing.T, directory, action string, extra ...string) *exec.Cmd {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestLocalEnvironmentHelper$")
	cmd.Dir = directory
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "STARHUB_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "STARHUB_LOCAL_ENV_ACTION="+action)
	cmd.Env = append(cmd.Env, extra...)
	return cmd
}

func TestLocalEnvironmentHelper(t *testing.T) {
	action := os.Getenv("STARHUB_LOCAL_ENV_ACTION")
	if action == "" {
		return
	}
	err := func() error {
		if err := config.LoadLocalEnvironment(); err != nil {
			return err
		}
		startup, err := config.LoadStartup(nil, os.Getenv)
		if err != nil {
			return err
		}
		if action == "operator" {
			if err := operator.OpenDatabase("starhub.db"); err != nil {
				return err
			}
		} else {
			if _, err := config.EnsurePersistentCryptoSecret(startup.Bootstrap.Database, action != "read-only"); err != nil {
				return err
			}
			if expected := os.Getenv("STARHUB_LOCAL_ENV_EXPECTED"); expected != "" && os.Getenv("STARHUB_CRYPTO_SECRET") != expected {
				return errors.New("environment precedence was not preserved")
			}
			if action == "prepare" || action == "read-only" {
				return nil
			}
			if err := common.ValidateCredentialConfiguration(); err != nil {
				return err
			}
			if err := model.InitDB(startup.Bootstrap.Database.DSN); err != nil {
				return err
			}
		}
		defer model.CloseDB()
		if action == "seed" {
			channel := &model.Channel{Name: "fixture", Type: 1, Key: "fixture-upstream-key", Models: "fixture-model", Group: "default", Status: 1, Weight: 1}
			if err := model.InsertChannel(channel); err != nil {
				return err
			}
			encrypted, err := common.Encrypt("fixture-customer-key")
			if err != nil {
				return err
			}
			return model.DB.Create(&model.Token{Name: "fixture", Key: encrypted, KeyHash: common.HashKey("fixture-customer-key"), Status: 1}).Error
		}
		channel, err := model.GetChannelByID(1)
		if err != nil || channel.Key != "fixture-upstream-key" {
			return errors.New("saved channel credential did not survive process restart")
		}
		token, err := model.GetTokenByKey("fixture-customer-key")
		if err != nil {
			return errors.New("customer lookup hash did not survive process restart")
		}
		plaintext, err := common.Decrypt(token.Key)
		if err != nil || plaintext != "fixture-customer-key" {
			return errors.New("saved customer credential did not survive process restart")
		}
		return nil
	}()
	if err != nil {
		fmt.Fprintln(os.Stdout, err)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "ready")
	os.Exit(0)
}

func TestLocalEnvironmentPersistsAcrossProcessesAndOperators(t *testing.T) {
	directory := t.TempDir()
	prefix := "# Keep configuration and comments\r\nSTARHUB_PORT=3109\r\nEXAMPLE='literal $HOME'\r\n"
	file := filepath.Join(directory, ".env")
	if err := os.WriteFile(file, []byte(prefix), 0600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"seed", "verify", "operator"} {
		output, err := localEnvCommand(t, directory, action).CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v: %s", action, err, output)
		}
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(content, []byte(prefix)) || !regexp.MustCompile(`\r\nSTARHUB_CRYPTO_SECRET=[0-9a-f]{64}\r\n$`).Match(content) || bytes.Count(content, []byte("STARHUB_CRYPTO_SECRET=")) != 1 {
		t.Fatal("initialization did not preserve the file and persist exactly one generated secret")
	}
	before := append([]byte(nil), content...)
	if output, err := localEnvCommand(t, directory, "verify").CombinedOutput(); err != nil {
		t.Fatalf("repeated restart failed: %v: %s", err, output)
	}
	content, _ = os.ReadFile(file)
	if !bytes.Equal(content, before) {
		t.Fatal("restart changed the saved .env")
	}
	entries, _ := filepath.Glob(filepath.Join(directory, ".env-init-*"))
	if len(entries) != 0 {
		t.Fatal("initialization left a temporary secret file")
	}
}

func TestLocalEnvironmentConcurrentFirstStartup(t *testing.T) {
	directory := t.TempDir()
	commands := make([]*exec.Cmd, 6)
	outputs := make([]bytes.Buffer, len(commands))
	for i := range commands {
		commands[i] = localEnvCommand(t, directory, "prepare")
		commands[i].Stdout = &outputs[i]
		commands[i].Stderr = &outputs[i]
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("parallel initialization failed: %v: %s", err, outputs[i].String())
		}
	}
	content, err := os.ReadFile(filepath.Join(directory, ".env"))
	if err != nil || !regexp.MustCompile(`^STARHUB_CRYPTO_SECRET=[0-9a-f]{64}\n$`).Match(content) {
		t.Fatal("parallel initialization did not converge on one persisted secret")
	}
	if _, err := os.Stat(filepath.Join(directory, ".env.lock")); !os.IsNotExist(err) {
		t.Fatal("initialization left its lock behind")
	}
}

func TestLocalEnvironmentPrioritiesAndInvalidFiles(t *testing.T) {
	for _, tc := range []struct {
		name, content, expectedError string
		extra                        []string
	}{
		{"process precedence", "STARHUB_CRYPTO_SECRET='file-fixture' # preserved\n", "", []string{"STARHUB_CRYPTO_SECRET=process-fixture", "STARHUB_LOCAL_ENV_EXPECTED=process-fixture"}},
		{"quoted value", "export STARHUB_CRYPTO_SECRET=\"file-fixture\" # preserved\n", "", []string{"STARHUB_LOCAL_ENV_EXPECTED=file-fixture"}},
		{"portable variable case", "starhub_crypto_secret=file-fixture\n", "", []string{"STARHUB_LOCAL_ENV_EXPECTED=file-fixture"}},
		{"empty secret", "STARHUB_CRYPTO_SECRET=\n", "empty or invalid", nil},
		{"duplicate", "STARHUB_CRYPTO_SECRET=fixture\nSTARHUB_CRYPTO_SECRET=other\n", "duplicate", nil},
		{"duplicate case", "STARHUB_CRYPTO_SECRET=fixture\nstarhub_crypto_secret=other\n", "duplicate", nil},
		{"comment as value", "STARHUB_CRYPTO_SECRET= # restore original\n", "empty or invalid", nil},
		{"unterminated quote", "STARHUB_CRYPTO_SECRET='fixture\n", "invalid", nil},
		{"malformed line", "must-not-be-displayed\n", "invalid", nil},
		{"control character", "STARHUB_CRYPTO_SECRET=\"fixture\\nsecret\"\n", "invalid", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			file := filepath.Join(directory, ".env")
			if err := os.WriteFile(file, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			output, err := localEnvCommand(t, directory, "prepare", tc.extra...).CombinedOutput()
			if (err != nil) != (tc.expectedError != "") || tc.expectedError != "" && !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("unexpected startup result: %v: %s", err, output)
			}
			if strings.Contains(string(output), "must-not-be-displayed") || strings.Contains(string(output), "file-fixture") {
				t.Fatal("startup output disclosed file contents")
			}
			content, _ := os.ReadFile(file)
			if string(content) != tc.content {
				t.Fatal("existing file was unexpectedly replaced")
			}
		})
	}
	directory := t.TempDir()
	if output, err := localEnvCommand(t, directory, "prepare", "STARHUB_CRYPTO_SECRET=environment-fixture").CombinedOutput(); err != nil {
		t.Fatalf("environment-only startup failed: %v: %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(directory, ".env")); !os.IsNotExist(err) {
		t.Fatal("environment configuration was copied to disk")
	}
}

func TestLocalEnvironmentRefusesCorruptionAndUnsafeFiles(t *testing.T) {
	for _, tc := range []struct{ name, expected string }{
		{"corrupt database", "could not inspect credential storage"},
		{"directory environment", "regular file"},
		{"oversized environment", "1 MiB"},
		{"no append capacity", "no space for a crypto secret"},
		{"held initialization lock", "initialization is locked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			switch tc.name {
			case "corrupt database":
				if err := os.WriteFile(filepath.Join(directory, "starhub.db"), []byte("not a SQLite database"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory environment":
				if err := os.Mkdir(filepath.Join(directory, ".env"), 0700); err != nil {
					t.Fatal(err)
				}
			case "oversized environment":
				if err := os.WriteFile(filepath.Join(directory, ".env"), bytes.Repeat([]byte("#"), (1<<20)+1), 0600); err != nil {
					t.Fatal(err)
				}
			case "no append capacity":
				if err := os.WriteFile(filepath.Join(directory, ".env"), bytes.Repeat([]byte("#\n"), 1<<19), 0600); err != nil {
					t.Fatal(err)
				}
			case "held initialization lock":
				if err := os.WriteFile(filepath.Join(directory, ".env.lock"), []byte("existing lock"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			output, err := localEnvCommand(t, directory, "prepare").CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.expected) {
				t.Fatalf("unsafe filesystem state was accepted: %v: %s", err, output)
			}
			if tc.name == "corrupt database" || tc.name == "held initialization lock" {
				if _, err := os.Stat(filepath.Join(directory, ".env")); !os.IsNotExist(err) {
					t.Fatal("unsafe initialization wrote a replacement secret")
				}
			}
			if tc.name == "held initialization lock" {
				content, err := os.ReadFile(filepath.Join(directory, ".env.lock"))
				if err != nil || string(content) != "existing lock" {
					t.Fatal("another process's lock was modified")
				}
			}
			if tc.name == "no append capacity" {
				content, err := os.ReadFile(filepath.Join(directory, ".env"))
				if err != nil || !bytes.Equal(content, bytes.Repeat([]byte("#\n"), 1<<19)) {
					t.Fatal("failed persistence modified the existing environment file")
				}
			}
		})
	}
}

func TestLocalEnvironmentProtectsExistingDatabaseAndRejectsUnsafeInitialization(t *testing.T) {
	for _, schema := range []string{
		`CREATE TABLE channels (id INTEGER PRIMARY KEY, "key" TEXT); INSERT INTO channels VALUES (1, 'enc:v2:legacy:fixture:fixture')`,
		`CREATE TABLE tokens (id INTEGER PRIMARY KEY, "key" TEXT, key_hash TEXT); INSERT INTO tokens VALUES (1, '', 'existing-hash')`,
		`CREATE TABLE sms_configurations (id INTEGER PRIMARY KEY, "key" TEXT); INSERT INTO sms_configurations VALUES (1, 'enc:v2:legacy:fixture:fixture')`,
	} {
		directory := t.TempDir()
		file := filepath.Join(directory, "starhub.db")
		db, err := sql.Open("sqlite", file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(schema); err != nil {
			t.Fatal(err)
		}
		db.Close()
		before, _ := os.ReadFile(file)
		output, err := localEnvCommand(t, directory, "prepare").CombinedOutput()
		if err == nil || !strings.Contains(string(output), "restore the original") {
			t.Fatalf("existing credential dependency was not protected: %v: %s", err, output)
		}
		after, _ := os.ReadFile(file)
		if !bytes.Equal(before, after) {
			t.Fatal("read-only inspection modified the existing database")
		}
		if _, err := os.Stat(filepath.Join(directory, ".env")); !os.IsNotExist(err) {
			t.Fatal("a replacement secret was written for an existing credential")
		}
	}
	for _, tc := range []struct {
		name, expected string
		extra          []string
	}{
		{"production", "only available in development", []string{"STARHUB_ENV=production"}},
		{"invalid mode", "only available in development", []string{"STARHUB_ENV=prodution"}},
		{"network", "configure STARHUB_CRYPTO_SECRET explicitly", []string{"STARHUB_DB_DRIVER=postgres", "STARHUB_DB_DSN=host=unreachable.invalid"}},
		{"SQLite URI", "configure STARHUB_CRYPTO_SECRET explicitly", []string{"STARHUB_DB=file:fixture.db?mode=rw"}},
		{"operator missing secret", "restore the original", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			action := "prepare"
			if tc.name == "operator missing secret" {
				action = "read-only"
			}
			output, err := localEnvCommand(t, directory, action, tc.extra...).CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.expected) {
				t.Fatalf("unsafe initialization was accepted: %v: %s", err, output)
			}
			if _, err := os.Stat(filepath.Join(directory, ".env")); !os.IsNotExist(err) {
				t.Fatal("unsafe initialization wrote a secret")
			}
		})
	}
}
