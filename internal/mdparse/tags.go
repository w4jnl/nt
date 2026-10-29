package mdparse

import (
	"regexp"
	"strings"
)

// Tag is a tag a note carries (docs/format.md §4): from the frontmatter's tags, or an inline #tag
// in the text outside code. Name is lowercase and has no #; / makes a hierarchy (work/q3).
type Tag struct {
	Name string
	Line int // 1-based source line
}

// inlineTag is a #tag word: after the line's start or a space, a letter, then letters, digits,
// -, _ and /. A heading's # has a space after it, a link's #anchor no space before it.
var inlineTag = regexp.MustCompile(`(?:^|\s)#(\p{L}[\p{L}\p{N}_/-]*)`)

// Tags reads the tags of note, the frontmatter's first, in order, each with its line. A tag
// written twice is there twice.
func Tags(note []byte) []Tag {
	lines := strings.Split(string(note), "\n")
	tags := frontTags(lines)
	inText := text(lines)
	for i, l := range lines {
		if !inText(i) {
			continue
		}
		for _, part := range outsideCode(l) {
			for _, m := range inlineTag.FindAllStringSubmatch(part, -1) {
				if name := tagName(m[1]); name != "" {
					tags = append(tags, Tag{Name: name, Line: i + 1})
				}
			}
		}
	}
	return tags
}

// frontTags reads the frontmatter's tags key in the forms notes have: a flow list
// (tags: [a, b]), a block list (tags: then - a lines), or one value (tags: a, or a, b).
func frontTags(lines []string) []Tag {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	var tags []Tag
	add := func(v string, line int) {
		for _, s := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			if name := tagName(unquote(s)); name != "" {
				tags = append(tags, Tag{Name: name, Line: line})
			}
		}
	}
	block := false
	for i := 1; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], "\r")
		if t := strings.TrimSpace(l); t == "---" || t == "..." {
			break
		}
		if block {
			if item, ok := strings.CutPrefix(strings.TrimSpace(l), "- "); ok {
				add(item, i+1)
				continue
			}
			block = false
		}
		key, v, ok := strings.Cut(l, ":")
		if !ok || key != "tags" {
			continue
		}
		v = strings.TrimSpace(v)
		switch {
		case v == "":
			block = true
		case strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]"):
			add(v[1:len(v)-1], i+1)
		default:
			add(v, i+1)
		}
	}
	return tags
}

// tagName is a tag as nt compares it: lowercase, without a leading # or a trailing /.
func tagName(s string) string {
	return strings.ToLower(strings.Trim(strings.TrimPrefix(strings.TrimSpace(s), "#"), "/"))
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// HasTag reports whether name is the tag q or under it: work/q3 is under work.
func HasTag(name, q string) bool {
	return name == q || strings.HasPrefix(name, q+"/")
}
