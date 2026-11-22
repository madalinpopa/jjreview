Source docs/demo/setup.tape

Type@300ms "jjj"
Sleep 500ms
Type "]"
Sleep 800ms
Type "["
Sleep 800ms
Type "J"
Sleep 1s
Type "K"
Sleep 1s

Type "d"
Sleep 1.2s
Type "d"
Sleep 600ms
Type "z"
Sleep 1s
Type "z"
Sleep 600ms

Type "f"
Sleep 1.5s
Type "f"
Sleep 1s

Type "/"
Sleep 500ms
Type "notfound"
Sleep 800ms
Enter
Wait+Screen@10s /notfound 1\/1/
Sleep 1.5s
Type "n"
Sleep 1.5s
Type "n"
Sleep 1.5s
Type "N"
Sleep 1.2s
Type "N"
Wait+Screen@10s /notfound 1\/1/
Sleep 1.2s
Escape
Sleep 1s

Type "s"
Sleep 1.5s
Type "k"
Sleep 800ms
Type "s"
Sleep 1.5s

Type "]"
Sleep 600ms
Type@250ms "jjjj"
Sleep 500ms
Type "v"
Sleep 600ms
Type@400ms "jjj"
Sleep 800ms
Type "s"
Sleep 1.5s

Type "J"
Sleep 1s
Type@300ms "lll"
Sleep 1.5s
Type@300ms "hhh"
Sleep 800ms

Tab
Sleep 600ms
Type " "
Sleep 1.5s

Type "c"
Wait+Screen@10s /New commit/
Sleep 1s
Type "fix: answer 404 for unknown codes"
Sleep 1.5s
Ctrl+S
Wait+Screen@30s /created/
Sleep 2s

Tab
Sleep 500ms
Type "K"
Sleep 2s

Type "R"
Sleep 1.5s

Type "?"
Sleep 2s
Escape
Sleep 1s

Type " "
Sleep 800ms
Type "q"
Wait+Screen@10s /press q again/
Sleep 1.5s
Type "q"
Sleep 2s
Type "jj log"
Enter
Sleep 4s
