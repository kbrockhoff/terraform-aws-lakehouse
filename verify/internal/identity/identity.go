package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// STSAPI is the STS subset the assumer needs.
type STSAPI interface {
	AssumeRole(ctx context.Context, in *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
}

// SessionName is the role session name for a persona. Persona trust policies require the
// lfverify-<run-id>- prefix, and CloudTrail attributes calls to the run through it.
func SessionName(runID, persona string) string {
	n := "lfverify-" + runID + "-" + persona
	if len(n) > 64 {
		n = n[:64]
	}
	return n
}

// Assumer assumes persona roles. Each persona gets one aws.Config whose credentials re-assume the
// role shortly before they expire, so clients built once stay valid for runs of any length.
type Assumer struct {
	STS   STSAPI
	Base  aws.Config
	RunID string
	// Policy, when set, is the inline session policy for every session (probe mode). A session
	// policy can only narrow the role's permissions.
	Policy string

	mu    sync.Mutex
	cache map[string]aws.Config
}

// refreshBefore renews credentials this long before they expire.
const refreshBefore = 5 * time.Minute

// Config returns an aws.Config that acts as the persona's role. The first call assumes the role,
// so a persona that cannot be assumed fails here rather than on its first API call.
func (a *Assumer) Config(ctx context.Context, persona, roleARN string) (aws.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cache == nil {
		a.cache = map[string]aws.Config{}
	}
	if cfg, ok := a.cache[persona]; ok {
		return cfg, nil
	}
	in := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String(SessionName(a.RunID, persona)),
		DurationSeconds: aws.Int32(3600),
	}
	if a.Policy != "" {
		in.Policy = aws.String(a.Policy)
	}
	// The provider assumes the role on every refresh; a provider that returned one fixed
	// AssumeRole response would leave clients with expired credentials after an hour
	provider := aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		out, err := a.STS.AssumeRole(ctx, in)
		if err != nil {
			return aws.Credentials{}, fmt.Errorf("assume %s (%s): %w", persona, roleARN, err)
		}
		cr := out.Credentials
		return aws.Credentials{
			AccessKeyID:     aws.ToString(cr.AccessKeyId),
			SecretAccessKey: aws.ToString(cr.SecretAccessKey),
			SessionToken:    aws.ToString(cr.SessionToken),
			Source:          "lfverify",
			CanExpire:       true,
			Expires:         aws.ToTime(cr.Expiration),
		}, nil
	})
	creds := aws.NewCredentialsCache(provider, func(o *aws.CredentialsCacheOptions) { o.ExpiryWindow = refreshBefore })
	if _, err := creds.Retrieve(ctx); err != nil {
		return aws.Config{}, err
	}
	cfg := a.Base.Copy()
	cfg.Credentials = creds
	a.cache[persona] = cfg
	return cfg, nil
}

// Statement is one IAM policy statement.
type Statement struct {
	Sid       string                       `json:"Sid"`
	Effect    string                       `json:"Effect"`
	Action    []string                     `json:"Action"`
	Resource  []string                     `json:"Resource"`
	Condition map[string]map[string]string `json:"Condition,omitempty"`
}

// Policy is an IAM policy document.
type Policy struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

// ReadOnlyActions are the only non-S3 actions a probe session may call (plan.md, Security).
var ReadOnlyActions = []string{
	"glue:Get*",
	"glue:Search*",
	"lakeformation:GetDataAccess",
	"lakeformation:Get*",
	"lakeformation:List*",
	"lakeformation:Search*",
	"athena:StartQueryExecution",
	"athena:GetQueryExecution",
	"athena:GetQueryResults",
	"athena:GetWorkGroup",
	"athena:StopQueryExecution",
}

// ProbeSessionPolicy builds the inline session policy for probe mode. Athena must write query
// results, so s3:PutObject is allowed only under the given results prefixes (arn:...:bucket/prefix*).
func ProbeSessionPolicy(resultsObjectARNs, resultsBucketARNs, kmsKeyARNs []string) (string, error) {
	p := Policy{Version: "2012-10-17", Statement: []Statement{
		{Sid: "ReadOnly", Effect: "Allow", Action: ReadOnlyActions, Resource: []string{"*"}},
	}}
	if len(resultsObjectARNs) > 0 {
		p.Statement = append(p.Statement, Statement{
			Sid: "QueryResults", Effect: "Allow", Action: []string{"s3:GetObject", "s3:PutObject"}, Resource: resultsObjectARNs,
		})
	}
	if len(resultsBucketARNs) > 0 {
		p.Statement = append(p.Statement, Statement{
			Sid: "QueryResultsBucket", Effect: "Allow", Action: []string{"s3:GetBucketLocation", "s3:ListBucket"}, Resource: resultsBucketARNs,
		})
	}
	if len(kmsKeyARNs) > 0 {
		p.Statement = append(p.Statement, Statement{
			Sid: "QueryResultsKey", Effect: "Allow", Action: []string{"kms:Decrypt", "kms:GenerateDataKey"}, Resource: kmsKeyARNs,
		})
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	if len(b) > 2048 {
		return "", fmt.Errorf("probe session policy is %d bytes; STS allows 2048 packed", len(b))
	}
	return string(b), nil
}
