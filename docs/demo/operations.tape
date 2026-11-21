Source docs/demo/setup.tape

Type "4"
Wait+Screen@10s /describe commit/
Sleep 2s

Type@600ms "jjj"
Sleep 500ms
Type@600ms "kk"
Sleep 800ms
Ctrl+D
Sleep 1s
Ctrl+U
Sleep 1s
Type "G"
Sleep 1s
Type "g"
Sleep 1s

Type "l"
Sleep 2s
Type "h"
Sleep 1s

Type "u"
Wait+Screen@10s /any other key cancels/
Sleep 2.5s
Type "y"
Wait+Screen@10s /undid operation/
Sleep 2.5s

Type "g"
Sleep 500ms
Type "j"
Sleep 1s
Type "r"
Wait+Screen@10s /any other key cancels/
Sleep 2.5s
Type "y"
Wait+Screen@10s /restored operation/
Sleep 2.5s

Type "g"
Sleep 500ms
Type@400ms "jjjjjj"
Sleep 800ms
Type "a"
Wait+Screen@10s /any other key cancels/
Sleep 2.5s
Type "y"
Wait+Screen@10s /abandoned operation/
Sleep 2.5s

Type "R"
Sleep 1.5s

Type "?"
Sleep 2s
Escape
Sleep 1s

Type "q"
Sleep 1.5s
