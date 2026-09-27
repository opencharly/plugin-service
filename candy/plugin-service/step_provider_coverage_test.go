package service

import (
	"testing"

	"github.com/opencharly/spec/spec"
)

// TestServiceVerb_MaterializeStep_IgnoresInertScalars pins the C7 ctx contract end-to-end: the
// service materializer consumes ONLY `runAsUser` (→ TargetScope) and `candyName` (→ provenance);
// `pkgFormat` and `distroTags` are inert for a packaged-unit step. It FAILS if the materializer
// starts reading either of the two scalars it is documented to ignore — the behaviour boundary
// this candy owns. (The sibling TestServiceVerb_StepProvider asserts the positive mapping; this is
// the negative half.)
func TestServiceVerb_MaterializeStep_IgnoresInertScalars(t *testing.T) {
	op := &spec.Op{PluginInput: map[string]any{"service": "nginx"}}

	a := (verb{}).MaterializeStep(op, "1000", "mylayer", "rpm", []string{"fedora:43", "fedora"})
	b := (verb{}).MaterializeStep(op, "1000", "mylayer", "deb", []string{"debian:13", "debian"})

	as, bs := a.(*spec.ServicePackagedStep), b.(*spec.ServicePackagedStep)
	if as.Unit != bs.Unit || as.Enable != bs.Enable || as.CandyName != bs.CandyName || as.TargetScope != bs.TargetScope {
		t.Fatalf("pkgFormat/distroTags changed the materialized step (%+v vs %+v) — service must consume only runAsUser + candyName", as, bs)
	}
}
