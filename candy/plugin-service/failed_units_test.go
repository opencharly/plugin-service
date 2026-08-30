package service

import (
	"context"
	"strings"
	"testing"

	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/spec"
)

func runService(t *testing.T, responses []fakeResponse, in map[string]any) kit.Result {
	t.Helper()
	cc := &fakeCC{exec: &fakeExec{responses: responses}}
	return verb{}.RunVerb(context.Background(), cc, &spec.Op{PluginInput: in})
}

func TestNoneFailed_CleanMachinePasses(t *testing.T) {
	res := runService(t, []fakeResponse{{matchPrefix: "--failed", stdout: "", exit: 0}},
		map[string]any{"none_failed": true})
	if res.Status != kit.StatusPass {
		t.Errorf("an empty --failed list must pass, got %+v", res)
	}
}

// The failure must NAME the units. "some unit failed" costs the reader another round trip
// to the guest, and on a disposable bed the guest is already destroyed by then.
func TestNoneFailed_NamesEveryOffender(t *testing.T) {
	out := "fprintd.service loaded failed failed Fingerprint Authentication\n" +
		"vendor-agent.service loaded failed failed Vendor Agent\n"
	res := runService(t, []fakeResponse{{matchPrefix: "--failed", stdout: out, exit: 0}},
		map[string]any{"none_failed": true})
	if res.Status != kit.StatusFail {
		t.Fatalf("two failed units must fail the check, got %+v", res)
	}
	for _, want := range []string{"fprintd.service", "vendor-agent.service", "2"} {
		if !strings.Contains(res.Message, want) {
			t.Errorf("the failure must report %q; got %q", want, res.Message)
		}
	}
}

func TestNoneFailed_IgnoreRegexExcludes(t *testing.T) {
	out := "fprintd.service loaded failed failed Fingerprint Authentication\n"
	res := runService(t, []fakeResponse{{matchPrefix: "--failed", stdout: out, exit: 0}},
		map[string]any{"none_failed": true, "ignore": "^fprintd"})
	if res.Status != kit.StatusPass {
		t.Errorf("the only offender was ignored, so the sweep must pass; got %+v", res)
	}
}

// An invalid regex must be an ERROR. Silently ignoring an unparseable pattern would mean
// the step asserts something other than what it says, with no signal.
func TestNoneFailed_BadIgnoreRegexFails(t *testing.T) {
	res := runService(t, []fakeResponse{{matchPrefix: "--failed", stdout: "", exit: 0}},
		map[string]any{"none_failed": true, "ignore": "([unclosed"})
	if res.Status != kit.StatusFail || !strings.Contains(res.Message, "not a valid regexp") {
		t.Errorf("an unparseable ignore must fail loudly, got %+v", res)
	}
}

// THE failure-open hazard. `systemctl --failed` exiting non-zero with no output is
// ambiguous — it can mean "nothing failed", or it can mean there is no systemd / no user
// manager / no bus to ask. Reading the second as the first turns the strongest whole-machine
// assertion into an unconditional pass.
func TestNoneFailed_UnreachableManagerFailsRatherThanPassing(t *testing.T) {
	// The values here are MEASURED, not invented. In a plain Arch container:
	//   $ systemctl --failed --no-legend --plain
	//   System has not been booted with systemd as init system (PID 1). Can't operate.
	//   [stderr; stdout empty] exit=1
	//   $ systemctl is-system-running
	//   offline
	//   exit=1
	// An earlier revision keyed on `exit == 127` and so passed VACUOUSLY here — the
	// check reported "no failed system units" on a machine with no systemd at all.
	res := runService(t, []fakeResponse{
		{matchPrefix: "--failed", stdout: "", exit: 1},
		{matchPrefix: "is-system-running", stdout: "offline\n", exit: 1},
	}, map[string]any{"none_failed": true, "scope": "user"})
	if res.Status != kit.StatusFail {
		t.Fatalf("an unreachable manager must FAIL, not pass as 'nothing failed'; got %+v", res)
	}
	if !strings.Contains(res.Message, "cannot reach") {
		t.Errorf("the message must say the manager was unreachable; got %q", res.Message)
	}
}

func TestNoneFailed_UnknownStateAlsoFails(t *testing.T) {
	res := runService(t, []fakeResponse{
		{matchPrefix: "--failed", stdout: "", exit: 1},
		{matchPrefix: "is-system-running", stdout: "unknown\n", exit: 1},
	}, map[string]any{"none_failed": true})
	if res.Status != kit.StatusFail {
		t.Errorf("is-system-running=unknown means the manager could not be asked; got %+v", res)
	}
}

func TestNoneFailed_NoAnswerAtAllFails(t *testing.T) {
	res := runService(t, []fakeResponse{
		{matchPrefix: "--failed", stdout: "", exit: 1},
		{matchPrefix: "is-system-running", stdout: "", exit: 1},
	}, map[string]any{"none_failed": true})
	if res.Status != kit.StatusFail {
		t.Errorf("an empty is-system-running answer must not be read as reachable; got %+v", res)
	}
}

// ...but a live manager that merely answers non-zero (degraded, starting) is reachable, and
// an empty --failed list from it is a genuine pass.
func TestNoneFailed_DegradedManagerStillCounts(t *testing.T) {
	res := runService(t, []fakeResponse{
		{matchPrefix: "--failed", stdout: "", exit: 1},
		{matchPrefix: "is-system-running", stdout: "degraded\n", exit: 1},
	}, map[string]any{"none_failed": true})
	if res.Status != kit.StatusPass {
		t.Errorf("a reachable-but-degraded manager reporting no failed units must pass; got %+v", res)
	}
}

func TestNoneFailed_UserScopeAsksTheUserManager(t *testing.T) {
	ex := &fakeExec{responses: []fakeResponse{{matchPrefix: "--failed", stdout: "", exit: 0}}}
	res := verb{}.RunVerb(context.Background(), &fakeCC{exec: ex},
		&spec.Op{PluginInput: map[string]any{"none_failed": true, "scope": "user"}})
	if res.Status != kit.StatusPass {
		t.Fatalf("expected pass, got %+v", res)
	}
	if !strings.Contains(res.Message, "user") {
		t.Errorf("the verdict must say which manager was swept; got %q", res.Message)
	}
}

// The two shapes are different subjects and must not be combined: a step naming a unit AND
// sweeping every unit would produce one verdict for two questions.
func TestService_NoneFailedAndServiceAreMutuallyExclusive(t *testing.T) {
	res := runService(t, nil, map[string]any{"service": "sshd", "none_failed": true})
	if res.Status != kit.StatusFail || !strings.Contains(res.Message, "two steps") {
		t.Errorf("naming a service alongside none_failed must be refused; got %+v", res)
	}
}

// A step with neither must be refused rather than silently passing — `service:` became
// optional to make room for the sweep, and an empty step is now syntactically possible.
func TestService_EmptyStepIsRefused(t *testing.T) {
	res := runService(t, nil, map[string]any{"running": true})
	if res.Status != kit.StatusFail {
		t.Errorf("a step naming no service and no sweep must fail, got %+v", res)
	}
}
