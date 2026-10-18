package templates

import (
	"os"
	"strings"
	"testing"
)

// The variables guide on docs.opendesk.sh lists every variable.
func TestDocsListEveryVariable(t *testing.T) {
	raw, err := os.ReadFile("../../docs/src/content/docs/guides/email-templates/variables.mdx")
	if err != nil {
		t.Skipf("docs not in this checkout: %v", err)
	}
	doc := string(raw)
	for _, v := range append(append([]Variable{}, Variables...), HistoryVariables...) {
		if !strings.Contains(doc, v.Key) {
			t.Errorf("docs/…/email-templates/variables.mdx does not mention %s", v.Key)
		}
	}
	for _, s := range Slots {
		if !strings.Contains(doc, "{{ "+s.Key+" }}") {
			t.Errorf("docs/…/email-templates/variables.mdx does not mention {{ %s }}", s.Key)
		}
	}
}
