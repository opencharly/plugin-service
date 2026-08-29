package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/spec"
)

// fakeResponse is a canned matchPrefix→output entry for fakeExec.
type fakeResponse struct {
	matchPrefix string
	stdout      string
	exit        int
}

// fakeExec is a kit.Executor returning canned RunCapture output by command prefix (the
// supervisorctl/systemctl probe). For the service-verb tests it EXECUTES the probe script via
// `sh -c` so the supervisorctl-state parsing actually runs (the probe is a multi-line script,
// not a single grep). A test can inject a fake supervisorctl on PATH (withFakeSupervisorctl) to
// simulate a specific program state.
type fakeExec struct{ responses []fakeResponse }

func (f *fakeExec) RunCapture(_ context.Context, cmd string) (string, string, int, error) {
	// Check for a canned response first (a test injecting a specific exit).
	for _, r := range f.responses {
		if strings.Contains(cmd, r.matchPrefix) {
			return r.stdout, "", r.exit, nil
		}
	}
	// Otherwise run the probe script via a real shell (so the awk state parsing executes).
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	exit := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		}
	}
	return string(out), "", exit, nil
}
func (f *fakeExec) Kind() string { return "container" }

// withFakeSupervisorctl creates a fake `supervisorctl` on PATH that returns the given status
// line for any `status <svc>` call, so the probe's state parsing executes against a real
// program state. Returns a cleanup func.
func withFakeSupervisorctl(t *testing.T, statusLine string) func() {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "supervisorctl")
	content := "#!/bin/sh\nif [ \"$1\" = \"status\" ]; then\n  echo '" + statusLine + "'\n  exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPATH := os.Getenv("PATH")
	os.Setenv("PATH", dir+":"+oldPATH)
	return func() { os.Setenv("PATH", oldPATH) }
}

// fakeCC is a fake kit.CheckContext exercising the service verb's Exec leg.
type fakeCC struct{ exec kit.Executor }

func (c *fakeCC) Exec() kit.Executor { return c.exec }
func (c *fakeCC) Mode() kit.RunMode  { return kit.ModeLive }
func (c *fakeCC) HTTPDo(context.Context, kit.HTTPRequest) (kit.HTTPResponse, error) {
	return kit.HTTPResponse{}, nil
}
func (c *fakeCC) ResolveEndpoint(context.Context, int) (string, error) { return "", nil }
func (c *fakeCC) ResolveGraphicsEndpoint(context.Context, string) (kit.GraphicsEndpoint, error) {
	return kit.GraphicsEndpoint{}, nil
}
func (c *fakeCC) ResolveImageLabel(context.Context, string) (string, error) { return "", nil }
func (c *fakeCC) DialTimeout() time.Duration                                { return 3 * time.Second }
func (c *fakeCC) Box() string                                               { return "" }
func (c *fakeCC) Instance() string                                          { return "" }
func (c *fakeCC) Distros() []string                                         { return nil }
func (c *fakeCC) AddBackground(int)                                         {}

// TestServiceVerb: running true/false. Relocated from charly/checkrun_verbs_test.go's
// TestRunner_Service (#55 decoupling cone, Batch D) — mirrors candy/plugin-port and
// candy/plugin-http's own test pattern (R3).
func TestServiceVerb(t *testing.T) {
	t.Run("running true via supervisorctl", func(t *testing.T) {
		cc := &fakeCC{exec: &fakeExec{responses: []fakeResponse{
			{matchPrefix: "supervisorctl status 'jupyter'", exit: 0},
		}}}
		res := verb{}.RunVerb(context.Background(), cc, &spec.Op{PluginInput: map[string]any{"service": "jupyter", "running": true}})
		if res.Status != kit.StatusPass {
			t.Errorf("expected pass, got %+v", res)
		}
	})

	t.Run("running mismatch", func(t *testing.T) {
		cc := &fakeCC{exec: &fakeExec{responses: []fakeResponse{
			{matchPrefix: "supervisorctl status 'jupyter'", exit: 1},
		}}}
		res := verb{}.RunVerb(context.Background(), cc, &spec.Op{PluginInput: map[string]any{"service": "jupyter", "running": true}})
		if res.Status != kit.StatusFail {
			t.Errorf("expected fail, got %+v", res)
		}
	})

	// opencharly/charly#456: a supervisord program in a TERMINAL state (FATAL) must FAIL
	// the running check — the bare `grep -q RUNNING` + systemctl fallback masked it (the
	// wrapped launcher unit reports "active" even when the supervisord child is dead).
	t.Run("running fails for FATAL supervisord state", func(t *testing.T) {
		cleanup := withFakeSupervisorctl(t, "gwd-parent FATAL Exited too quickly (process log may have details)")
		defer cleanup()
		cc := &fakeCC{exec: &fakeExec{}}
		res := verb{}.RunVerb(context.Background(), cc, &spec.Op{PluginInput: map[string]any{"service": "gwd-parent", "running": true}})
		if res.Status != kit.StatusFail {
			t.Errorf("expected FAIL for a FATAL program, got %+v", res)
		}
	})

	t.Run("running passes for RUNNING supervisord state", func(t *testing.T) {
		cleanup := withFakeSupervisorctl(t, "keepalive RUNNING pid 7, uptime 0:00:42")
		defer cleanup()
		cc := &fakeCC{exec: &fakeExec{}}
		res := verb{}.RunVerb(context.Background(), cc, &spec.Op{PluginInput: map[string]any{"service": "keepalive", "running": true}})
		if res.Status != kit.StatusPass {
			t.Errorf("expected PASS for a RUNNING program, got %+v", res)
		}
	})

	t.Run("running passes for STARTING supervisord state (transient)", func(t *testing.T) {
		cleanup := withFakeSupervisorctl(t, "cstream-hyprland STARTING")
		defer cleanup()
		cc := &fakeCC{exec: &fakeExec{}}
		res := verb{}.RunVerb(context.Background(), cc, &spec.Op{PluginInput: map[string]any{"service": "cstream-hyprland", "running": true}})
		if res.Status != kit.StatusPass {
			t.Errorf("expected PASS for a STARTING (transient) program, got %+v", res)
		}
	})

	t.Run("running falls back to systemd when supervisorctl knows no such program", func(t *testing.T) {
		// No fake supervisorctl on PATH → the probe falls to systemctl. Use a service name
		// that exists on NO host (so the systemctl fallback also fails).
		cc := &fakeCC{exec: &fakeExec{}}
		res := verb{}.RunVerb(context.Background(), cc, &spec.Op{PluginInput: map[string]any{"service": "charly-nonexistent-svc-xyz", "running": true}})
		if res.Status != kit.StatusFail {
			t.Errorf("expected FAIL when the service is not running anywhere, got %+v", res)
		}
	})
}

// TestServiceVerb_RenderProvisionScript: the ACT role renders the enable + start shell
// under whichever init the live target runs. Relocated from
// charly/plugin_service_relocated_test.go's TestRelocatedServiceVerb_DispatchesViaKit (the
// act-role behavior half; the dispatch wiring stays in charly).
func TestServiceVerb_RenderProvisionScript(t *testing.T) {
	script, ok := verb{}.RenderProvisionScript(&spec.Op{PluginInput: map[string]any{"service": "nginx"}}, nil)
	if !ok || !strings.Contains(script, "systemctl enable") || !strings.Contains(script, "supervisorctl") {
		t.Fatalf("act: want an enable shell, got ok=%v %q", ok, script)
	}
}

// TestServiceVerb_StepProvider: the TYPED-STEP role names the ServicePackaged step kind
// and decodes plugin_input into the StepDescriptor the host materializer consumes.
// Relocated from charly/plugin_service_relocated_test.go's
// TestRelocatedServiceVerb_DispatchesViaKit (the step-role behavior half; the dispatch
// wiring + the materializer stay in charly).
func TestServiceVerb_StepProvider(t *testing.T) {
	got := verb{}.StepKind()
	if got != kit.StepKindServicePackaged {
		t.Fatalf("StepKind = %v, want StepKindServicePackaged", got)
	}
	desc := verb{}.ConstructStepDescriptor(&spec.Op{PluginInput: map[string]any{"service": "nginx"}})
	if desc.ServicePackaged == nil || desc.ServicePackaged.Unit != "nginx" || !desc.ServicePackaged.Enable {
		t.Fatalf("step descriptor = %+v", desc)
	}
}
