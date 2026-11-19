Require jj
Require go

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 620
Set Padding 24
Set TypingSpeed 45ms
Set WindowBar Colorful

Hide
Type `root=$(docs/demo-setup.sh) && go build -o "$root/bin/jjr" ./cmd/jjr && export PS1='> ' PATH="$root/bin:$PATH" JJ_CONFIG="$root/jj.toml" && cd "$root/repo" && clear`
Enter
Wait@60s
Show

Type "jj diff --stat"
Enter
Sleep 2s
Type "jjr"
Enter
Sleep 2s

Type@600ms "jj"
Sleep 1s
Type "h"
Sleep 500ms
Type "j"
Sleep 800ms
Type "l"
Sleep 800ms
Type "d"
Sleep 1.5s
Type " "
Sleep 1s

Type "h"
Sleep 500ms
Type "j"
Sleep 800ms
Type " "
Sleep 1s

Type "c"
Wait+Screen@60s /generated from/
Sleep 3s
Ctrl+S
Wait+Screen@30s /created/
Sleep 2s

Type "k"
Sleep 800ms
Type "l"
Sleep 500ms
Type@400ms "jjjjjjjjj"
Sleep 800ms
Type " "
Sleep 800ms
Type "h"
Sleep 500ms
Type "jj"
Sleep 800ms
Type " "
Sleep 1s

Type "c"
Wait+Screen@60s /generated from/
Sleep 3s
Ctrl+S
Wait+Screen@30s /created/
Sleep 2s

Type@500ms "kkk"
Sleep 800ms
Type "l"
Sleep 500ms
Type "f"
Sleep 2.5s

Type "q"
Sleep 500ms
Type "jj log"
Enter
Sleep 4s
