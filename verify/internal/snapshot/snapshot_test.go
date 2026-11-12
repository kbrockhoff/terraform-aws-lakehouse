package snapshot

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
	"path/filepath"
	"slices"
	"testing"
)

func TestDiff(t *testing.T) {
	before := &State{Tables: []string{"c/alpha.orders metadata=1"}, Permissions: []string{"p1"}, Settings: []string{"s"}}
	after := &State{Tables: []string{"c/alpha.orders metadata=2"}, Permissions: []string{"p1", "p2"}, Settings: []string{"s"}}
	got := Diff(before, after)
	want := []string{"- table c/alpha.orders metadata=1", "+ table c/alpha.orders metadata=2", "+ permission p2"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if d := Diff(before, before); len(d) != 0 {
		t.Errorf("identical snapshots must not differ: %v", d)
	}
}

func TestSaveLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "before.json")
	s := &State{LFTags: []string{"b", "a"}}
	s.sort()
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	back, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(back.LFTags, []string{"a", "b"}) || len(Diff(s, back)) != 0 {
		t.Errorf("round trip: %+v", back)
	}
}

func TestSettingsOrderIgnored(t *testing.T) {
	p := func(ids ...string) []lftypes.DataLakePrincipal {
		var out []lftypes.DataLakePrincipal
		for _, id := range ids {
			out = append(out, lftypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(id)})
		}
		return out
	}
	a := canon(sortedSettings(&lftypes.DataLakeSettings{DataLakeAdmins: p("b", "a")}))
	b := canon(sortedSettings(&lftypes.DataLakeSettings{DataLakeAdmins: p("a", "b")}))
	if a != b {
		t.Errorf("admin order must not matter:\n%s\n%s", a, b)
	}
	if c := canon(sortedSettings(&lftypes.DataLakeSettings{DataLakeAdmins: p("a")})); c == a {
		t.Error("a removed admin must still differ")
	}
}
