package config

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type withDefault struct {
	Name string `env:"CONFIG_TEST_NAME,required"`
	Port int    `env:"CONFIG_TEST_PORT" envDefault:"8080"`
}

func (c *withDefault) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port out of range")
	}
	return nil
}

func TestLoadMissingVarNames(t *testing.T) {
	type needTwo struct {
		A string `env:"CONFIG_TEST_MISSING_A,required"`
		B string `env:"CONFIG_TEST_MISSING_B,required"`
	}
	_, err := Load[needTwo]()
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "CONFIG_TEST_MISSING_A") || !strings.Contains(msg, "CONFIG_TEST_MISSING_B") {
		t.Fatalf("error should list missing vars, got %s", msg)
	}
}

func TestLoadDefaultsAndValidate(t *testing.T) {
	t.Setenv("CONFIG_TEST_NAME", "board")
	cfg, err := Load[withDefault]()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "board" || cfg.Port != 8080 {
		t.Fatalf("got %+v", cfg)
	}

	t.Setenv("CONFIG_TEST_PORT", "0")
	_, err = Load[withDefault]()
	if err == nil || !strings.Contains(err.Error(), "port out of range") {
		t.Fatalf("expected validate error, got %v", err)
	}
}

func TestMustLoadSuccess(t *testing.T) {
	t.Setenv("CONFIG_TEST_NAME", "gateway")
	cfg := MustLoad[withDefault]()
	if cfg.Name != "gateway" || cfg.Port != 8080 {
		t.Fatalf("got %+v", cfg)
	}
}

func TestMustLoadMissingVarExits(t *testing.T) {
	if os.Getenv("CONFIG_MUSTLOAD_HELPER") == "1" {
		MustLoad[struct {
			A string `env:"CONFIG_MUSTLOAD_MISSING_XYZ,required"`
		}]()
		return
	}

	// Relaunch this test binary. os.Args[0] is the test process, not external input.
	cmd := exec.Command(os.Args[0], "-test.run=^TestMustLoadMissingVarExits$") //nolint:gosec // G204
	env := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CONFIG_MUSTLOAD_MISSING_XYZ=") {
			env = append(env, e)
		}
	}
	cmd.Env = append(env, "CONFIG_MUSTLOAD_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected process to exit, output: %s", out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit, err=%v output=%s", err, out)
	}
	if !strings.Contains(string(out), "CONFIG_MUSTLOAD_MISSING_XYZ") {
		t.Fatalf("exit message should name the missing var, output: %s", out)
	}
}
