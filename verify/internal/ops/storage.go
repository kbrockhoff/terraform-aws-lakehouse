package ops

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/manifest"
)

// StorageRead tries to bypass the permission service by reading a table's storage directly
// with the persona's own credentials (FR-022). The first failing call decides the outcome; if
// the metadata lookup unexpectedly succeeds, the object read is still attempted and recorded.
func (c *Clients) StorageRead(ctx context.Context, t manifest.Target) Result {
	loc, err := c.S3Tables.GetTableMetadataLocation(ctx, &s3tables.GetTableMetadataLocationInput{
		TableBucketARN: aws.String(t.TableBucketARN), Namespace: aws.String(t.Database), Name: aws.String(t.Table),
	})
	if err != nil {
		return fromErr(err, classify.Exists{Resource: true})
	}
	bucket, key, err := splitS3URI(aws.ToString(loc.MetadataLocation))
	if err != nil {
		return Result{Outcome: classify.Error, Evidence: classify.Evidence{Message: err.Error()}}
	}
	out, err := c.S3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		res := fromErr(err, classify.Exists{Resource: true})
		res.Evidence.Message = "metadata location readable, object denied: " + res.Evidence.Message
		return res
	}
	out.Body.Close()
	return Result{Outcome: classify.Allowed, Evidence: classify.Evidence{Message: "table storage readable directly"}}
}

func splitS3URI(uri string) (bucket, key string, err error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "s3" || u.Host == "" {
		return "", "", fmt.Errorf("not an s3 URI: %q", uri)
	}
	return u.Host, strings.TrimPrefix(u.Path, "/"), nil
}
