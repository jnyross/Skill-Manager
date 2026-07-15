package setup_test

import (
	"testing"

	"github.com/jnyross/Skill-Manager/internal/catalog"
	"github.com/jnyross/Skill-Manager/internal/setup"
)

func TestCompareDriftIgnoresRevisionMovementWhenBoundaryIsIdentical(t *testing.T) {
	member := reviewedMember()
	resolved := setup.BoundaryEvidence{
		Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Subpath:  member.Source.Subpath, ContentSHA256: member.Source.ContentSHA256,
		LicenseSHA256: member.License.NoticeSHA256, Scripts: member.Scripts, Executables: member.Executables,
		MetadataSHA256: member.Source.MetadataSHA256, DependencyEvidenceSHA256: member.Source.DependencyEvidenceSHA256,
		ExternalActionEvidenceSHA256: member.Source.ExternalActionEvidenceSHA256,
	}
	review := setup.CompareDrift(member, resolved)
	if review.Material || len(review.Changes) != 0 {
		t.Fatalf("byte-identical revision movement classified material: %#v", review)
	}
}

func TestCompareDriftClassifiesEveryGovernedSurface(t *testing.T) {
	member := reviewedMember()
	tests := []struct {
		name  string
		edit  func(*setup.BoundaryEvidence)
		class setup.DriftClass
	}{
		{"source boundary", func(e *setup.BoundaryEvidence) { e.Subpath = "skills/renamed" }, setup.DriftSourceBoundary},
		{"content", func(e *setup.BoundaryEvidence) { e.ContentSHA256 = digest('c') }, setup.DriftContent},
		{"license", func(e *setup.BoundaryEvidence) { e.LicenseSHA256 = digest('d') }, setup.DriftLicense},
		{"scripts", func(e *setup.BoundaryEvidence) { e.Scripts = []string{"scripts/new.sh"} }, setup.DriftScripts},
		{"executable modes", func(e *setup.BoundaryEvidence) { e.Executables = nil }, setup.DriftExecutableMode},
		{"metadata", func(e *setup.BoundaryEvidence) { e.MetadataSHA256 = digest('e') }, setup.DriftMetadata},
		{"dependencies", func(e *setup.BoundaryEvidence) { e.DependencyEvidenceSHA256 = digest('f') }, setup.DriftDependencies},
		{"external actions", func(e *setup.BoundaryEvidence) { e.ExternalActionEvidenceSHA256 = digest('1') }, setup.DriftExternalAction},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := setup.BoundaryEvidence{Subpath: member.Source.Subpath, ContentSHA256: member.Source.ContentSHA256, LicenseSHA256: member.License.NoticeSHA256, Scripts: member.Scripts, Executables: member.Executables, MetadataSHA256: member.Source.MetadataSHA256, DependencyEvidenceSHA256: member.Source.DependencyEvidenceSHA256, ExternalActionEvidenceSHA256: member.Source.ExternalActionEvidenceSHA256}
			test.edit(&evidence)
			review := setup.CompareDrift(member, evidence)
			if !review.Material || !review.Has(test.class) {
				t.Fatalf("review = %#v, want class %q", review, test.class)
			}
		})
	}
}

func reviewedMember() catalog.Member {
	return catalog.Member{
		Name:    "probe",
		Source:  catalog.Source{Subpath: "skills/probe", ReviewedRevision: digest('a')[:40], ContentSHA256: digest('a'), MetadataSHA256: digest('m'), DependencyEvidenceSHA256: digest('n'), ExternalActionEvidenceSHA256: digest('o')},
		License: catalog.License{NoticeSHA256: digest('b')},
		Scripts: []string{"scripts/check.sh"}, Executables: []string{"scripts/check.sh"},
	}
}

func digest(char byte) string {
	result := make([]byte, 64)
	for i := range result {
		result[i] = char
	}
	return string(result)
}
