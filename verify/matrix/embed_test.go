package matrix

import (
	"bytes"
	"os"
	"testing"

	parsed "github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
)

func TestProbeExampleEmbedded(t *testing.T) {
	b, err := os.ReadFile("probe.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ProbeExample, b) {
		t.Error("embedded template differs from probe.example.yaml")
	}
	m, err := parsed.Parse(ProbeExample)
	if err != nil {
		t.Fatalf("embedded template must be a valid matrix: %v", err)
	}
	if m.Mode != parsed.Probe {
		t.Errorf("mode %s, want probe", m.Mode)
	}
}
