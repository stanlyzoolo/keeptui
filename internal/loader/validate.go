package loader

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

func Validate(data []byte) (Tool, []ValidationError) {
	var t Tool
	var errs []ValidationError

	if err := yaml.Unmarshal(data, &t); err != nil {
		return t, []ValidationError{{fmt.Sprintf("invalid YAML: %v", err)}}
	}

	if t.Name == "" {
		errs = append(errs, ValidationError{`missing field "name"`})
	} else if strings.Contains(t.Name, "/") || strings.Contains(t.Name, "\\") || strings.Contains(t.Name, "..") {
		errs = append(errs, ValidationError{`field "name" must not contain path separators or ".."`})
	}
	if len(t.Categories) == 0 && len(t.CommandGroups) == 0 {
		errs = append(errs, ValidationError{`missing field "categories" or "command_groups" (at least one required)`})
	}

	for _, cat := range t.Categories {
		for bi, b := range cat.Bindings {
			if b.Key == "" {
				errs = append(errs, ValidationError{
					fmt.Sprintf("binding #%d in %q: empty key", bi+1, cat.Name),
				})
			}
			if b.Desc == "" {
				errs = append(errs, ValidationError{
					fmt.Sprintf("binding #%d in %q: empty desc", bi+1, cat.Name),
				})
			}
		}
	}

	for _, cg := range t.CommandGroups {
		for ci, c := range cg.Commands {
			if c.Cmd == "" {
				errs = append(errs, ValidationError{
					fmt.Sprintf("command #%d in %q: empty cmd", ci+1, cg.Name),
				})
			}
			if c.Desc == "" {
				errs = append(errs, ValidationError{
					fmt.Sprintf("command #%d in %q: empty desc", ci+1, cg.Name),
				})
			}
		}
	}

	return t, errs
}
