Source docs/demo/setup.tape

Type@300ms "jjjj"
Sleep 500ms
Type@300ms "kk"
Sleep 800ms
Type "]"
Sleep 1s
Type "["
Sleep 1s
Ctrl+D
Sleep 1s
Ctrl+U
Sleep 1s
Type "G"
Sleep 1s
Type "g"
Sleep 1s

Type "J"
Sleep 1s
Type "K"
Sleep 1s

Tab
Sleep 800ms
Type@700ms "jj"
Sleep 500ms
Type "k"
Sleep 800ms
Type "G"
Sleep 800ms
Type "g"
Sleep 800ms
Type "j"
Sleep 800ms
Tab
Sleep 1s

Type "d"
Sleep 1.5s
Type "d"
Sleep 800ms
Type "D"
Sleep 1.5s
Type "D"
Sleep 1s

Type "z"
Sleep 1s
Type "z"
Sleep 800ms
Type "Z"
Sleep 1s
Type "Z"
Sleep 1s

Type "f"
Sleep 1.5s
Type "G"
Sleep 1s
Type "f"
Sleep 1.5s

Type "S"
Sleep 1.5s
Type "]"
Sleep 800ms
Type " "
Sleep 1.5s

Tab
Sleep 500ms
Type "j"
Sleep 800ms
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

Type "g"
Sleep 800ms
Type "e"
Sleep 2s
Type ":q"
Enter
Sleep 1.5s

Type "R"
Sleep 1.5s

Type "?"
Sleep 2s
Type@400ms "jjjjjj"
Sleep 1s
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
