Require jj
Require go
Require git

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 620
Set Padding 24
Set TypingSpeed 45ms
Set WindowBar Colorful

Hide
Sleep 1s
Type `root=$(docs/demo/setup.sh) && go build -o "$root/bin/jjr" ./cmd/jjr && export PS1='> ' PATH="$root/bin:$PATH" JJ_CONFIG="$root/jj.toml" XDG_CONFIG_HOME="$root/config" EDITOR=vi && cd "$root/repo" && clear || exit 1`
Enter
Wait@60s
Show

Type "jjr"
Enter
Wait+Screen@10s /main\.go/
Sleep 1.5s
