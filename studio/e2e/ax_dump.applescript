-- Dumps the accessibility tree of a Studio window by walking it depth-first
-- (System Events' `entire contents` silently drops parts of WebKit trees).
-- One line per element that carries a name, description or value, plus
-- pop-up buttons and images: role|name|description|value|x|y|w|h
-- Usage: osascript ax_dump.applescript <process name>
on run argv
  set theName to item 1 of argv
  tell application "System Events"
    tell process theName
      return my walk(window 1, 0)
    end tell
  end tell
end run

on walk(e, depth)
  set out to ""
  if depth > 40 then return out
  tell application "System Events"
    try
      set r to role of e
      set n to ""
      set d to ""
      set v to ""
      try
        set n to name of e as text
      end try
      try
        set d to description of e as text
      end try
      try
        set v to value of e as text
      end try
      if n is "missing value" then set n to ""
      if d is "missing value" then set d to ""
      if v is "missing value" then set v to ""
      if (n & d & v) is not "" or r is "AXPopUpButton" or r is "AXImage" then
        set p to position of e
        set s to size of e
        set out to out & r & "|" & n & "|" & d & "|" & v & "|" & (item 1 of p) & "|" & (item 2 of p) & "|" & (item 1 of s) & "|" & (item 2 of s) & linefeed
      end if
      -- Monaco's editor subtree is huge and exposes nothing useful; skip it.
      if r is "AXTextArea" then return out
      repeat with c in UI elements of e
        set out to out & my walk(c, depth + 1)
      end repeat
    end try
  end tell
  return out
end walk
