package mystery

import (
	"embed"
	"os"
	"path/filepath"
)

// caseFS embeds the shipped mystery case so the binary is self-contained.
// A mysteries/case01 directory next to the working data dir still overrides
// the embedded copy (owner-authored cases, live edits without a rebuild).
//
//go:embed cases/case01
var caseFS embed.FS

// loadCaseFiles returns (triggersJSON, answerSHA). A complete case on disk
// wins (operators can hot-swap a case); otherwise the embedded copy.
func loadCaseFiles(caseDir string) ([]byte, []byte) {
	tj, err1 := os.ReadFile(filepath.Join(caseDir, "triggers.json"))
	as, err2 := os.ReadFile(filepath.Join(caseDir, "answer.sha256"))
	if err1 == nil && err2 == nil {
		return tj, as
	}
	tj, err1 = caseFS.ReadFile("cases/case01/triggers.json")
	as, err2 = caseFS.ReadFile("cases/case01/answer.sha256")
	if err1 != nil || err2 != nil {
		return nil, nil
	}
	return tj, as
}
