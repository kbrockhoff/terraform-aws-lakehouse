package preflight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
)

// S3TablesCatalogFile records, in the run's output directory, whether s3tablescatalog existed when
// the run started. Teardown uses it to prove the catalog survived (spec 003 FR-030).
const S3TablesCatalogFile = "s3tablescatalog.before"

// S3TablesCatalogExists reports whether the federated s3tablescatalog exists for the caller.
func S3TablesCatalogExists(ctx context.Context, g *glue.Client) (bool, error) {
	_, err := g.GetCatalog(ctx, &glue.GetCatalogInput{CatalogId: aws.String("s3tablescatalog")})
	var notFound *gluetypes.EntityNotFoundException
	switch {
	case errors.As(err, &notFound):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// RecordS3TablesCatalog writes whether the catalog exists now, unless a record already exists: a
// rerun of preflight in the same run keeps the original.
func RecordS3TablesCatalog(path string, exists bool) error {
	if _, err := os.Stat(path); err == nil { // #nosec G703 -- path is under the run's validated output directory
		return nil
	}
	return os.WriteFile(path, []byte(strconv.FormatBool(exists)+"\n"), 0o600) // #nosec G703 -- path is under the run's validated output directory
}

// CheckS3TablesCatalogKept fails when teardown left no s3tablescatalog although the catalog must
// still exist: it existed before the run, or the run deployed (its manifest was written, so the
// fixture's account settings created it). A run interrupted before deploying may legitimately end
// without one.
func CheckS3TablesCatalogKept(exists bool, recordPath, manifestPath string) error {
	if exists {
		return nil
	}
	before := false
	if b, err := os.ReadFile(recordPath); err == nil { // #nosec G304 G703 -- path is under the run's validated output directory
		before = strings.TrimSpace(string(b)) == "true"
	}
	_, statErr := os.Stat(manifestPath) // #nosec G703 -- path is under the run's validated output directory
	deployed := statErr == nil
	if before || deployed {
		return fmt.Errorf("s3tablescatalog no longer exists after teardown (existed before the run: %t, run deployed: %t); nothing may delete it (spec 003 FR-030)", before, deployed)
	}
	return nil
}
