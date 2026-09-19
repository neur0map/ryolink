package config

import (
	"os"
	"path/filepath"
	"testing"
)

func minimalYAML() string {
	return `
ryolink:
  name: "t"
  domain: "t.example"
owner:
  name: "o"
  fingerprint: "SHA256:x"
rooms:
  - name: lounge
    type: chat
`
}

func writeLoad(t *testing.T, body string) *Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ryolink.yaml")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg
}

func TestDefaultsPopulate(t *testing.T) {
	cfg := writeLoad(t, minimalYAML())
	if cfg.Server.Port != 2222 || cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("server defaults wrong: %+v", cfg.Server)
	}
	if cfg.Server.WebPort != 8090 || cfg.Server.WebBind != "127.0.0.1" {
		t.Fatalf("web defaults wrong: %+v", cfg.Server)
	}
	if cfg.WebAudioEnabled() {
		t.Fatal("web audio must default off")
	}
	sec := cfg.Security
	if sec.MaxConns != 512 || sec.MaxConnsPerIP != 4 || sec.NewConnsPerMin != 30 ||
		sec.MaxAuthFails != 10 || sec.BanMinutes != 30 || sec.ProbeBanMinutes != 60 {
		t.Fatalf("security defaults wrong: %+v", sec)
	}
	// data dir defaults to the config file's directory
	if cfg.Server.DataDir != filepath.Dir(filepath.Join(t.TempDir(), "ryolink.yaml")) &&
		filepath.Base(cfg.Server.DataDir) == "" {
		t.Fatalf("data dir not defaulted: %q", cfg.Server.DataDir)
	}
	if cfg.Server.DataDir == "" {
		t.Fatal("data dir must never be empty")
	}
}

func TestSectionsOverride(t *testing.T) {
	cfg := writeLoad(t, minimalYAML()+`
server:
  port: 22
  web_audio: true
security:
  max_conns: 7
  allow_cidrs: ["10.0.0.0/8", "1.2.3.4"]
`)
	if cfg.Server.Port != 22 {
		t.Fatalf("port override lost: %d", cfg.Server.Port)
	}
	if !cfg.WebAudioEnabled() {
		t.Fatal("web_audio: true lost")
	}
	if cfg.Security.MaxConns != 7 {
		t.Fatal("security override lost")
	}
	if len(cfg.Security.AllowCIDRs) != 2 {
		t.Fatal("allow list lost")
	}
}

func TestInvalidCIDRRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ryolink.yaml")
	os.WriteFile(p, []byte(minimalYAML()+`
security:
  deny_cidrs: ["not-a-cidr"]
`), 0600)
	if _, err := Load(p); err == nil {
		t.Fatal("invalid deny CIDR must fail validation")
	}
}

func TestSecretEnvWins(t *testing.T) {
	cfg := writeLoad(t, minimalYAML()+"\napi:\n  openai_key: \"file-key\"\n")
	if got := cfg.Secret("OPENAI_API_KEY", cfg.API.OpenAIKey); got != "file-key" {
		t.Fatalf("file value should be used when env unset, got %q", got)
	}
	t.Setenv("OPENAI_API_KEY", "env-key")
	if got := cfg.Secret("OPENAI_API_KEY", cfg.API.OpenAIKey); got != "env-key" {
		t.Fatalf("env must win, got %q", got)
	}
}
