package ops

import "testing"

func TestSplitS3URI(t *testing.T) {
	b, k, err := splitS3URI("s3://abc--table-s3/metadata/00001-x.metadata.json")
	if err != nil || b != "abc--table-s3" || k != "metadata/00001-x.metadata.json" {
		t.Errorf("got %s %s %v", b, k, err)
	}
	if _, _, err := splitS3URI("https://x/y"); err == nil {
		t.Error("non-s3 URI must fail")
	}
}
