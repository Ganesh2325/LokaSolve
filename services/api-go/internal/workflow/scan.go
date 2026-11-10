package workflow

import "context"

// ScanResult is the outcome of a malware scanner.
// Status "not_scanned" means no scanner ran. It is not a clean verdict.
type ScanResult struct {
	Status string
}

const (
	ScanNotScanned = "not_scanned"
	ScanClean      = "clean"
	ScanRejected   = "rejected"
)

// MalwareScanner inspects stored bytes. A nil scanner and UnconfiguredScanner
// both mean scanning is not available.
type MalwareScanner interface {
	Scan(ctx context.Context, contentType string, body []byte) (ScanResult, error)
}

// UnconfiguredScanner records that no malware scanner is connected.
// A nil error does not mean the file is safe.
type UnconfiguredScanner struct{}

func (UnconfiguredScanner) Scan(context.Context, string, []byte) (ScanResult, error) {
	return ScanResult{Status: ScanNotScanned}, nil
}
