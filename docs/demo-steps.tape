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
Sleep 1s
Type `root=$(docs/demo-setup.sh) && go build -o "$root/bin/jjr" ./cmd/jjr && export PS1='> ' PATH="$root/bin:$PATH" JJ_CONFIG="$root/jj.toml" XDG_CONFIG_HOME="$root/config" && cd "$root/repo" && clear || exit 1`
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
Tab
Sleep 500ms
Type "j"
Sleep 800ms
Tab
Sleep 800ms
Type "d"
Sleep 1.5s
Type " "
Sleep 1s

Tab
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
Tab
Sleep 500ms
Type@400ms "jjjjjjjjj"
Sleep 800ms
Type " "
Sleep 800ms
Tab
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
Tab
Sleep 500ms
Type "f"
Sleep 2.5s

Type "2"
Wait+Screen@10s /add url shortener/
Sleep 1s
Type@600ms "jj"
Sleep 800ms
Type "l"
Wait+Screen@10s /A server.handler_test\.go/
Sleep 1.5s
Type "h"
Sleep 800ms
Type "/"
Sleep 500ms
Type@120ms "shortener"
Sleep 1.5s
Enter
Sleep 1s
Enter
Wait+Screen@10s /@ \w+ +feat: add url shortener/
Sleep 2.5s
Type "2"
Sleep 1s
Type "1"
Sleep 1s

Type "q"
Sleep 500ms
Type "jj log"
Enter
Sleep 4s
