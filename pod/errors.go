package pod

import (
	"sort"
	"strings"
)

// ValidationError is a single problem found in a manifest or bundle, attributed
// to the offending field path (e.g. "spec.tools[0].credentials_ref").
type ValidationError struct {
	Field string
	Msg   string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Msg
	}
	return e.Field + ": " + e.Msg
}

// ValidationErrors aggregates all problems found in a single pass, so a caller
// sees every issue at once rather than one-at-a-time.
type ValidationErrors []ValidationError

func (es ValidationErrors) Error() string {
	switch len(es) {
	case 0:
		return "no validation errors"
	case 1:
		return es[0].Error()
	}
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}

// ErrorOrNil returns a non-nil error only when there is at least one problem,
// for the idiomatic `if err := v.ErrorOrNil(); err != nil` pattern.
func (es ValidationErrors) ErrorOrNil() error {
	if len(es) == 0 {
		return nil
	}
	return es
}

// collector accumulates ValidationErrors during a validation pass.
type collector struct {
	errs ValidationErrors
}

func (c *collector) add(field, msg string) {
	c.errs = append(c.errs, ValidationError{Field: field, Msg: msg})
}

func (c *collector) result() ValidationErrors {
	sort.SliceStable(c.errs, func(i, j int) bool {
		return c.errs[i].Field < c.errs[j].Field
	})
	return c.errs
}
