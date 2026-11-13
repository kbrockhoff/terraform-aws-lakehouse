// Package matrix holds the expectation matrices as files, and embeds the probe template so a
// standalone lfverify can write it without a repository checkout (spec 002, FR-034).
package matrix

import _ "embed"

// ProbeExample is probe.example.yaml.
//
//go:embed probe.example.yaml
var ProbeExample []byte
