package templates

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/useopendesk/opendesk/internal/httpx"
)

// Refuse turns problems into the 422 a save answers, each problem in errors at field, or
// nil when there are none.
func Refuse(field string, probs []Problem) error {
	if len(probs) == 0 {
		return nil
	}
	e := httpx.Validation(probs[0].Message)
	for _, p := range probs {
		e.Errors = append(e.Errors, &huma.ErrorDetail{Message: p.Message, Location: "body." + field, Value: p.Token})
	}
	return e
}

// CheckSaved refuses a template of kind with problems; fields are the workspace's custom
// field variables.
func CheckSaved(field, src, kind string, fields map[string]bool) error {
	return Refuse(field, Check(src, kind, fields))
}
