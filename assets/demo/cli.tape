# The command line, about 15 s: a line in today's journal, a todo, the todo list, a search, a
# folder. record.sh fills in @D@ (the scene) and @BIN@.
Output frames/
Set Shell bash
Set FontSize 15
Set Width 1100
Set Height 560
Set Padding 14
Set Framerate 20
Set TypingSpeed 55ms
Set CursorBlink false
Set Theme { "name": "nt", "background": "#14181A", "foreground": "#F2F5F5", "cursor": "#3FD0D4", "selection": "#173437", "black": "#0F1214", "red": "#F0645A", "green": "#5FC47A", "yellow": "#E8963C", "blue": "#12999D", "magenta": "#3FD0D4", "cyan": "#3FD0D4", "white": "#F2F5F5", "brightBlack": "#5E6D70", "brightRed": "#F0645A", "brightGreen": "#5FC47A", "brightYellow": "#E8963C", "brightBlue": "#3FD0D4", "brightMagenta": "#3FD0D4", "brightCyan": "#3FD0D4", "brightWhite": "#FFFFFF" }
Env NT_CONFIG "@D@/config.toml"
Env NT_STATE "@D@/state"
Env PATH "@BIN@:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"

Hide
Type "cd @D@/notes && PS1='$ ' && clear" Enter
Sleep 0.5s
Show
Type "nt a 'Ordered the new fan'" Enter
Sleep 1.6s
Type "nt todo add 'Book the dentist' --due fri" Enter
Sleep 1.6s
Type "nt todo" Enter
Sleep 2.8s
Type "nt s restic" Enter
Sleep 3s
Type "clear && nt ls homelab" Enter
Sleep 2.6s
