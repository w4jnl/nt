package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// command is one command of the public CLI, for the completion scripts: its names, what it does,
// its flags and what its arguments are.
type command struct {
	names []string
	help  string
	flags []flag
	args  []string // per position: folder, note, dir, a word list "(a b)", or "" for free text
}

type flag struct {
	name, help string
	value      string // "" for a switch, else what the value is: a word list "(a b)" or free text
}

var jsonFlag = flag{"--json", "machine-readable output", ""}

var commands = []command{
	{[]string{"t", "today"}, "open today's journal in $EDITOR", []flag{jsonFlag}, nil},
	{[]string{"a", "add"}, "add a timestamped line to today's journal", []flag{jsonFlag}, nil},
	{[]string{"n", "new"}, "a new dated note, linked from today's journal", []flag{jsonFlag}, []string{"", "folder"}},
	{[]string{"f", "find"}, "find a note with fzf and open it", nil, nil},
	{[]string{"s", "search"}, "search the notes with rg", []flag{jsonFlag}, nil},
	{[]string{"ls"}, "the folders and notes in a folder", []flag{jsonFlag}, []string{"folder"}},
	{[]string{"view"}, "show one note rendered", []flag{{"--theme", "light or dark", "(auto dark light)"}, {"--images", "draw images", "(auto on off)"}}, []string{"note"}},
	{[]string{"paste"}, "save the clipboard's image beside a note", []flag{{"--name", "what the image shows", "name"}, jsonFlag}, []string{"note"}},
	{[]string{"init"}, "make a folder a notes repository", []flag{{"--clone", "clone this repository", "url"}, jsonFlag}, []string{"dir"}},
	{[]string{"sync"}, "commit, merge and push now", []flag{jsonFlag}, nil},
	{[]string{"install"}, "the background sync job or the nvim plugin", []flag{{"--sync", "nt sync every sync.interval", ""}, {"--nvim", "nt's nvim plugin", ""}}, nil},
	{[]string{"uninstall"}, "remove the sync job or the nvim plugin", []flag{{"--sync", "the background sync job", ""}, {"--nvim", "nt's nvim plugin", ""}}, nil},
	{[]string{"doctor"}, "check the repository, git, the timer and tools", []flag{jsonFlag}, nil},
	{[]string{"import"}, "bring an Obsidian vault over once", []flag{{"--dry-run", "show the plan, write nothing", ""}, jsonFlag}, []string{"(obsidian)", "dir"}},
	{[]string{"theme"}, "nt's preview style for glow", []flag{{"--glamour", "write glamour-dark.json and glamour-light.json", ""}}, []string{"dir"}},
	{[]string{"completion"}, "print a shell completion script", nil, []string{"(bash zsh)"}},
	{[]string{"version"}, "print the version", nil, nil},
	{[]string{"help"}, "show usage", nil, nil},
}

const completionHelp = `usage: nt completion <bash|zsh>
  bash: echo 'eval "$(nt completion bash)"' >> ~/.bashrc
  zsh:  echo 'eval "$(nt completion zsh)"'  >> ~/.zshrc   (after compinit), or
        nt completion zsh > ~/.zfunc/_nt  with ~/.zfunc in $fpath before compinit
`

// nt completion bash|zsh: the completion script. It completes commands, flags and their values,
// and the repository's folders and notes, which it asks nt _complete for.
func runCompletion(args []string) int {
	if len(args) != 1 || args[0] != "bash" && args[0] != "zsh" {
		fmt.Fprint(os.Stderr, completionHelp)
		return 2
	}
	if args[0] == "bash" {
		fmt.Print(bashCompletion())
	} else {
		fmt.Print(zshCompletion())
	}
	return 0
}

// nt _complete folders|notes: the repository's folders, or its notes, as repository paths, one
// per line. Hidden folders and assets/ are left out. Nothing when there is no repository.
func runComplete(args []string) int {
	if len(args) != 1 || args[0] != "folders" && args[0] != "notes" {
		return 2
	}
	v, _, err := openVault()
	if err != nil {
		return 0
	}
	var out []string
	_ = filepath.WalkDir(v.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == v.Root {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || d.IsDir() && name == "assets" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() == (args[0] == "folders") && (d.IsDir() || strings.HasSuffix(name, ".md")) {
			out = append(out, v.RelPath(p))
		}
		return nil
	})
	sort.Strings(out)
	for _, p := range out {
		fmt.Println(p)
	}
	return 0
}

func allNames() []string {
	var names []string
	for _, c := range commands {
		names = append(names, c.names...)
	}
	return names
}

func bashCompletion() string {
	var b strings.Builder
	b.WriteString(`# bash completion for nt — install with: eval "$(nt completion bash)"
_nt() {
    local cur prev cmd
    COMPREPLY=()
    cur=${COMP_WORDS[COMP_CWORD]}
    prev=${COMP_WORDS[COMP_CWORD-1]}
    cmd=${COMP_WORDS[1]}
    if [ "$COMP_CWORD" -eq 1 ]; then
`)
	fmt.Fprintf(&b, "        COMPREPLY=( $(compgen -W \"%s $(nt _complete folders 2>/dev/null)\" -- \"$cur\") )\n", strings.Join(allNames(), " "))
	b.WriteString("        return 0\n    fi\n    case \"$prev\" in\n")
	seen := map[string]bool{}
	for _, c := range commands {
		for _, f := range c.flags {
			if f.value == "" || seen[f.name] {
				continue
			}
			seen[f.name] = true
			if words, ok := wordList(f.value); ok {
				fmt.Fprintf(&b, "        %s) COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") ); return 0 ;;\n", f.name, words)
			} else {
				fmt.Fprintf(&b, "        %s) return 0 ;;\n", f.name)
			}
		}
	}
	b.WriteString("    esac\n    case \"$cmd\" in\n")
	for _, c := range commands {
		if len(c.flags) == 0 && len(c.args) == 0 {
			continue
		}
		fmt.Fprintf(&b, "        %s)\n", strings.Join(c.names, "|"))
		if len(c.flags) > 0 {
			var fs []string
			for _, f := range c.flags {
				fs = append(fs, f.name)
			}
			fmt.Fprintf(&b, "            if [[ $cur == -* ]]; then COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") ); return 0; fi\n", strings.Join(fs, " "))
		}
		if len(c.args) > 0 {
			// the position among the arguments, flags and their values left out
			b.WriteString("            local i n=0\n            for (( i=2; i<COMP_CWORD; i++ )); do\n")
			fmt.Fprintf(&b, "                case ${COMP_WORDS[i]} in %s) (( i++ )) ;; -*) ;; *) (( n++ )) ;; esac\n", strings.Join(valueFlags(), "|"))
			b.WriteString("            done\n            case $n in\n")
			for i, a := range c.args {
				if a == "" {
					continue
				}
				fmt.Fprintf(&b, "                %d) %s ;;\n", i, bashArg(a))
			}
			b.WriteString("            esac\n")
		}
		b.WriteString("            ;;\n")
	}
	b.WriteString("    esac\n    return 0\n}\ncomplete -o default -F _nt nt\n")
	return b.String()
}

func bashArg(a string) string {
	switch a {
	case "folder":
		return `COMPREPLY=( $(compgen -W "$(nt _complete folders 2>/dev/null)" -- "$cur") )`
	case "note":
		return `COMPREPLY=( $(compgen -W "$(nt _complete notes 2>/dev/null)" -- "$cur") )` // else files (-o default)
	case "dir":
		return `COMPREPLY=( $(compgen -d -- "$cur") )`
	}
	words, _ := wordList(a)
	return fmt.Sprintf(`COMPREPLY=( $(compgen -W "%s" -- "$cur") )`, words)
}

func zshCompletion() string {
	var b strings.Builder
	b.WriteString(`#compdef nt
# zsh completion for nt — install with: eval "$(nt completion zsh)" (after compinit),
# or save as _nt in a directory on $fpath before compinit.
_nt_folders() {
    local -a folders
    folders=(${(f)"$(nt _complete folders 2>/dev/null)"})
    _wanted folders expl folder compadd -a folders
}
_nt_notes() {
    local -a notes
    notes=(${(f)"$(nt _complete notes 2>/dev/null)"})
    _alternative 'notes:note:compadd -a notes' 'files:file:_files -g "*.md"'
}
_nt() {
    local -a cmds
    cmds=(
`)
	for _, c := range commands {
		for _, n := range c.names {
			fmt.Fprintf(&b, "        %s\n", zshQuote(n+":"+c.help))
		}
	}
	b.WriteString(`    )
    if (( CURRENT == 2 )); then
        _describe -t commands 'nt command' cmds
        _nt_folders
        return
    fi
    local cmd=${words[2]}
    shift words
    (( CURRENT-- ))
    case $cmd in
`)
	for _, c := range commands {
		if len(c.flags) == 0 && len(c.args) == 0 {
			continue
		}
		var specs []string
		for _, f := range c.flags {
			s := f.name + "[" + f.help + "]"
			switch words, ok := wordList(f.value); {
			case ok:
				s += ":" + strings.TrimPrefix(f.name, "--") + ":(" + words + ")"
			case f.value != "":
				s += ":" + f.value + ": "
			}
			specs = append(specs, zshQuote(s))
		}
		for i, a := range c.args {
			spec := fmt.Sprintf("%d:", i+1)
			switch a {
			case "":
				spec += "text: "
			case "folder":
				spec += "folder:_nt_folders"
			case "note":
				spec += "note:_nt_notes"
			case "dir":
				spec += "folder:_files -/"
			default:
				words, _ := wordList(a)
				spec += "word:(" + words + ")"
			}
			specs = append(specs, zshQuote(spec))
		}
		fmt.Fprintf(&b, "        %s) _arguments %s ;;\n", strings.Join(c.names, "|"), strings.Join(specs, " "))
	}
	b.WriteString("    esac\n}\n")
	b.WriteString("if [ \"${funcstack[1]}\" = _nt ]; then\n    _nt \"$@\"\nelse\n    compdef _nt nt\nfi\n")
	return b.String()
}

// valueFlags are the flags that take a value, which is not an argument.
func valueFlags() []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range commands {
		for _, f := range c.flags {
			if f.value != "" && !seen[f.name] {
				seen[f.name] = true
				out = append(out, f.name)
			}
		}
	}
	return out
}

// wordList reads "(a b c)" as the words "a b c".
func wordList(s string) (string, bool) {
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		return s[1 : len(s)-1], true
	}
	return "", false
}

// zshQuote single-quotes s for zsh.
func zshQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
