package identity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
)

type fakeSTS struct {
	calls []*sts.AssumeRoleInput
	exp   time.Time
}

func (f *fakeSTS) AssumeRole(_ context.Context, in *sts.AssumeRoleInput, _ ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	f.calls = append(f.calls, in)
	return &sts.AssumeRoleOutput{Credentials: &types.Credentials{
		AccessKeyId: aws.String("AKIA"), SecretAccessKey: aws.String("s"), SessionToken: aws.String("t"), Expiration: aws.Time(f.exp),
	}}, nil
}

func TestSessionName(t *testing.T) {
	if got := SessionName("abc123", "reader_multi"); got != "lfverify-abc123-reader_multi" {
		t.Errorf("got %s", got)
	}
	if got := SessionName("abc123", strings.Repeat("x", 80)); len(got) != 64 {
		t.Errorf("session names are capped at 64 characters, got %d", len(got))
	}
}

func TestConfigCachesAndRefreshes(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	f := &fakeSTS{exp: now.Add(time.Hour)}
	a := &Assumer{STS: f, RunID: "abc123", Policy: `{"x":1}`, now: func() time.Time { return now }}
	ctx := context.Background()
	if _, err := a.Config(ctx, "admin", "arn:aws:iam::1:role/a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Config(ctx, "admin", "arn:aws:iam::1:role/a"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("second call must hit the cache, got %d AssumeRole calls", len(f.calls))
	}
	in := f.calls[0]
	if aws.ToString(in.RoleSessionName) != "lfverify-abc123-admin" || aws.ToString(in.Policy) != `{"x":1}` || aws.ToInt32(in.DurationSeconds) != 3600 {
		t.Errorf("assume input: %+v", in)
	}
	now = now.Add(56 * time.Minute) // inside the refresh window
	if _, err := a.Config(ctx, "admin", "arn:aws:iam::1:role/a"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 {
		t.Errorf("credentials near expiry must refresh, got %d calls", len(f.calls))
	}
}

// TestProbeSessionPolicyIsReadOnly guards FR-027: the only write action a probe session holds
// is s3:PutObject on query results prefixes.
func TestProbeSessionPolicyIsReadOnly(t *testing.T) {
	doc, err := ProbeSessionPolicy(
		[]string{"arn:aws:s3:::results/sales/*"},
		[]string{"arn:aws:s3:::results"},
		[]string{"arn:aws:kms:us-east-1:1:key/k"},
	)
	if err != nil {
		t.Fatal(err)
	}
	var p Policy
	if err := json.Unmarshal([]byte(doc), &p); err != nil {
		t.Fatal(err)
	}
	mutating := []string{"Put", "Create", "Delete", "Update", "Grant", "Revoke", "Add", "Remove", "Batch", "Register", "Tag"}
	for _, s := range p.Statement {
		if s.Effect != "Allow" {
			continue
		}
		for _, a := range s.Action {
			verb := a[strings.Index(a, ":")+1:]
			for _, m := range mutating {
				if strings.HasPrefix(verb, m) && a != "s3:PutObject" {
					t.Errorf("statement %s allows mutating action %s", s.Sid, a)
				}
			}
			if a == "s3:PutObject" && strings.Join(s.Resource, ",") != "arn:aws:s3:::results/sales/*" {
				t.Errorf("s3:PutObject must be limited to results prefixes, got %v", s.Resource)
			}
		}
	}
}
