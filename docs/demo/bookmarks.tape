Source docs/demo/setup.tape

Type "3"
Wait+Screen@10s /main/
Sleep 2s

Type "n"
Wait+Screen@10s /New bookmark/
Sleep 1s
Type@150ms "fix-404"
Sleep 800ms
Tab
Sleep 800ms
Ctrl+U
Type@150ms "@-"
Sleep 800ms
Shift+Tab
Sleep 800ms
Enter
Wait+Screen@10s /saved fix-404/
Sleep 2s

Type "j"
Sleep 800ms
Type "k"
Sleep 800ms
Type "G"
Sleep 800ms
Type "g"
Sleep 1s

Type "G"
Sleep 800ms
Type "e"
Wait+Screen@10s /Edit bookmark/
Sleep 1s
Tab
Sleep 800ms
Ctrl+U
Type@150ms "@-"
Sleep 800ms
Enter
Wait+Screen@10s /saved main/
Sleep 2.5s

Type "g"
Sleep 800ms
Enter
Wait+Screen@10s /UNIFIED DIFF/
Sleep 2.5s
Type "3"
Sleep 1.5s

Type "e"
Wait+Screen@10s /Edit bookmark/
Sleep 1s
Ctrl+U
Type@150ms "fix-not-found"
Sleep 800ms
Tab
Sleep 500ms
Ctrl+U
Type@150ms "@---"
Sleep 800ms
Enter
Sleep 2.5s
Enter
Wait+Screen@10s /saved fix-not-found/
Sleep 2.5s

Type "d"
Wait+Screen@10s /Delete bookmark/
Sleep 2s
Type "y"
Wait+Screen@10s /deleted fix-not-found/
Sleep 2s

Type "R"
Sleep 1.5s

Type "?"
Sleep 2s
Escape
Sleep 1s

Type "q"
Sleep 1.5s
