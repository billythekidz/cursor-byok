### How to get the token?
**macos**
```bash
sqlite3 "$HOME/Library/Application Support/Cursor/User/globalStorage/state.vscdb" \
  "SELECT value FROM ItemTable WHERE key = 'cursorAuth/accessToken';"

```
**Windows**
```bash
sqlite3 "$env:APPDATA\Cursor\User\globalStorage\state.vscdb" "SELECT value FROM ItemTable WHERE key = 'cursorAuth/accessToken';"