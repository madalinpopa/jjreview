Source docs/demo/setup.tape

Type "2"
Wait+Screen@10s /add a makefile/
Sleep 1.5s
Type@600ms "jjj"
Sleep 500ms
Type@600ms "kk"
Sleep 800ms
Type "G"
Sleep 1s
Type "g"
Sleep 1s

Type "jj"
Sleep 800ms
Type "l"
Sleep 2s
Type "h"
Sleep 1s

Type "/"
Sleep 500ms
Type@150ms "chore"
Sleep 1s
Enter
Sleep 1.5s
Escape
Sleep 1s

Type "g"
Sleep 500ms
Type "jjjj"
Sleep 800ms
Enter
Wait+Screen@10s /1 Review/
Sleep 2.5s
Type "@"
Sleep 2s
Type "2"
Sleep 1.5s

Type "g"
Sleep 500ms
Type@600ms "jj"
Sleep 600ms
Type " "
Sleep 800ms
Type "j"
Sleep 600ms
Type " "
Sleep 1s
Type "s"
Wait+Screen@10s /y squash/
Sleep 2.5s
Type "y"
Wait+Screen@30s /squashed/
Sleep 2.5s

Type " "
Sleep 1s
Escape
Sleep 1s

Type "l"
Sleep 1.5s
Type "j"
Sleep 600ms
Type "l"
Sleep 1.5s
Type "R"
Sleep 1.5s

Type "g"
Sleep 500ms
Type "j"
Sleep 800ms
Type "a"
Wait+Screen@10s /any other key cancels/
Sleep 2.5s
Type "y"
Wait+Screen@30s /abandoned/
Sleep 2.5s

Type "f"
Wait+Screen@30s /fetched/
Sleep 2s

Type "b"
Wait+Screen@10s /new bookmark…/
Sleep 1.5s
Enter
Wait+Screen@10s /New bookmark/
Sleep 800ms
Type@150ms "fix-404"
Sleep 800ms
Enter
Wait+Screen@10s /saved fix-404/
Sleep 2s

Type "P"
Wait+Screen@10s /any other key cancels/
Sleep 2.5s
Type "y"
Wait+Screen@30s /pushed/
Sleep 2.5s

Ctrl+R
Sleep 1.5s

Type "?"
Sleep 2s
Escape
Sleep 1s

Type "q"
Sleep 1.5s
