package tldr

import (
	"bufio"
	"strings"

	"github.com/lepeshko/keys/internal/loader"
)

// Parse converts a tldr Markdown page into CommandGroups.
// tldr format: pairs of "- description:" + "`command`" lines.
func Parse(content string) []loader.CommandGroup {
	var commands []loader.Command
	var pendingDesc string

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "- ") {
			pendingDesc = strings.TrimSuffix(strings.TrimPrefix(line, "- "), ":")
			continue
		}

		if strings.HasPrefix(line, "`") && strings.HasSuffix(line, "`") && pendingDesc != "" {
			cmd := strings.Trim(line, "`")
			commands = append(commands, loader.Command{
				Cmd:  cmd,
				Desc: pendingDesc,
			})
			pendingDesc = ""
			continue
		}

		// reset pending if we see something that's not a command block
		if line != "" && !strings.HasPrefix(line, "`") {
			pendingDesc = ""
		}
	}

	if len(commands) == 0 {
		return nil
	}

	return []loader.CommandGroup{{
		Name:     "Examples",
		Commands: commands,
	}}
}
