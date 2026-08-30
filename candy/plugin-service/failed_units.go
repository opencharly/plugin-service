package service

// failed_units.go implements the `none_failed:` sweep — "no unit is in the failed state",
// for either systemd manager.
//
// Why it is not expressible per-unit: every other assertion this verb makes names a unit,
// and the value of this one is precisely that it names none. The unit that breaks a machine
// is the one nobody thought to write a check for, so an assertion that only covers the
// units somebody enumerated cannot catch it. `systemctl --failed` covers the set.
//
// Why it lives on `service:` rather than in a new verb: it is the same subject (systemd /
// supervisord unit state) reached through the same executor, and a second systemd verb
// would be two implementations of "ask the init system", which is what R3 exists to stop.

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/shellquote"

	"github.com/opencharly/plugin-service/candy/plugin-service/params"
)

// sweepFailedUnits asserts that `systemctl [--user] --failed` lists nothing, minus any
// unit matching the step's `ignore:` regex.
func sweepFailedUnits(ctx context.Context, cc kit.CheckContext, in params.ServiceInput, userFlag string) kit.Result {
	// --no-legend drops the header and the "N loaded units listed" footer; --plain drops
	// the ●/× status glyph that would otherwise land in field 1. What remains is one unit
	// name per line, or nothing at all.
	probe := fmt.Sprintf("systemctl%s --failed --no-legend --plain 2>/dev/null", userFlag)
	out, _, exit, err := cc.Exec().RunCapture(ctx, probe)
	if err != nil {
		return kit.Failf("failed-units probe: %v", err)
	}
	// A non-zero exit with no output is systemctl reporting "no failed units" on some
	// versions; a non-zero exit WITH output means the command itself did not work (no
	// systemd, no user manager, no bus). Distinguishing them matters: treating the second
	// as "nothing failed" is the assertion failing OPEN, which is the whole hazard this
	// check exists to close.
	trimmed := strings.TrimSpace(out)
	if exit != 0 && trimmed == "" {
		// Ambiguous, and dangerously so: an empty non-zero result can mean "nothing
		// failed" OR "there is no manager to ask". systemctl writes the latter to
		// STDERR — `System has not been booted with systemd as init system (PID 1).
		// Can't operate.` — leaving stdout empty and exiting 1, which is
		// indistinguishable from success by exit code alone.
		//
		// So ask `is-system-running`, and read its OUTPUT rather than its exit code.
		// It exits non-zero for `degraded`, `starting` and `maintenance` too, all of
		// which are live managers; only `offline` and `unknown` mean the question could
		// not be asked. Measured in a plain Arch container: `offline`, exit 1 — an
		// earlier revision keyed on `exit == 127` and therefore passed VACUOUSLY on
		// every container without systemd, which is the exact failure-open this check
		// exists to prevent.
		probe := fmt.Sprintf("systemctl%s is-system-running 2>/dev/null", userFlag)
		aliveOut, _, _, aliveErr := cc.Exec().RunCapture(ctx, probe)
		state := strings.TrimSpace(aliveOut)
		if aliveErr != nil || state == "" || state == "offline" || state == "unknown" {
			reported := state
			if reported == "" {
				reported = "no answer"
			}
			return kit.Failf("none_failed: cannot reach the %s systemd manager "+
				"(is-system-running: %s) — `systemctl%s --failed` exited %d with no "+
				"output, which is NOT the same as 'nothing failed'",
				scopeName(in.Scope), reported, userFlag, exit)
		}
	}

	var offenders []string
	var ignore *regexp.Regexp
	if in.Ignore != "" {
		re, err := regexp.Compile(in.Ignore)
		if err != nil {
			return kit.Failf("none_failed: ignore %q is not a valid regexp: %v", in.Ignore, err)
		}
		ignore = re
	}
	for _, line := range strings.Split(trimmed, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		unit := fields[0]
		if ignore != nil && ignore.MatchString(unit) {
			continue
		}
		offenders = append(offenders, unit)
	}

	want := in.NoneFailed != nil && *in.NoneFailed
	got := len(offenders) == 0
	if got != want {
		if want {
			// Name every offender. "some unit failed" costs the reader another round
			// trip to the guest, and on a disposable bed the guest is already gone.
			return kit.Failf("none_failed: %d %s unit(s) failed: %s",
				len(offenders), scopeName(in.Scope), strings.Join(offenders, ", "))
		}
		return kit.Fail("none_failed: false asserts that something IS failed, and nothing is")
	}
	return kit.Passf("no failed %s units", scopeName(in.Scope))
}

func scopeName(scope string) string {
	if scope == "user" {
		return "user"
	}
	return "system"
}

// userScopedUnitProbe returns a probe that asks ONLY the user manager, skipping the
// supervisorctl leg. supervisord has no user scope, so consulting it for a user unit finds
// nothing and falls through — the right answer by accident. It would be the WRONG answer if
// a supervisord program happened to share the unit's name, which on a desktop pod running
// both is not far-fetched.
func userScopedUnitProbe(action, unit string) string {
	return fmt.Sprintf("systemctl --user %s --quiet %s", action, shellquote.ShellQuote(unit))
}
