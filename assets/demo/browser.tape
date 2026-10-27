# The browser, about 25 s: a pinned folder, a note rendered, f and a letter to follow a link, b
# back, the todo list, the calendar, a search. record.sh fills in @D@ (the scene) and @BIN@.
Output frames/
Set Shell bash
Set FontSize 15
Set Width 1300
Set Height 720
Set Padding 14
Set Framerate 20
Set TypingSpeed 90ms
Set CursorBlink false
Set Theme { "name": "nt", "background": "#14181A", "foreground": "#F2F5F5", "cursor": "#3FD0D4", "selection": "#173437", "black": "#0F1214", "red": "#F0645A", "green": "#5FC47A", "yellow": "#E8963C", "blue": "#12999D", "magenta": "#3FD0D4", "cyan": "#3FD0D4", "white": "#F2F5F5", "brightBlack": "#5E6D70", "brightRed": "#F0645A", "brightGreen": "#5FC47A", "brightYellow": "#E8963C", "brightBlue": "#3FD0D4", "brightMagenta": "#3FD0D4", "brightCyan": "#3FD0D4", "brightWhite": "#FFFFFF" }
Env NT_CONFIG "@D@/config.toml"
Env NT_STATE "@D@/state"
Env PATH "@BIN@:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"

Hide
Type "cd @D@/notes && clear && nt" Enter
Sleep 2s
Show
Sleep 1.5s
Type "1"
Sleep 1.2s
Type "j"
Sleep 0.5s
Type "j"
Sleep 2.8s
Type "f"
Sleep 1.6s
Type "a"
Sleep 2.6s
Type "b"
Sleep 1.4s
Type "x"
Sleep 3s
Escape
Sleep 0.4s
Type "c"
Sleep 1.6s
Type "h"
Sleep 2.4s
Escape
Sleep 0.4s
Type "/"
Sleep 0.3s
Type "restic"
Sleep 2.8s
Escape
Sleep 1s
