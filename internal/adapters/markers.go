package adapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// UpsertSection replaces (or appends) the managed section for a skill inside
// a shared markdown file. Everything outside the markers is left untouched.
func UpsertSection(path, skill, content string) error {
	begin := fmt.Sprintf("<!-- shuhari:skill:%s:begin -->", skill)
	end := fmt.Sprintf("<!-- shuhari:skill:%s:end -->", skill)
	section := begin + "\n" + strings.TrimRight(content, "\n") + "\n" + end

	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return os.WriteFile(path, []byte(section+"\n"), 0o644)
	}
	if err != nil {
		return err
	}
	s := string(b)
	if i, j := strings.Index(s, begin), strings.Index(s, end); i >= 0 && j > i {
		s = s[:i] + section + s[j+len(end):]
	} else {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n" + section + "\n"
	}
	return os.WriteFile(path, []byte(s), 0o644)
}
