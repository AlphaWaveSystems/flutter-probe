-- Picks an item from the Nth pop-up button of a Studio window by label prefix.
-- WebKit renders <select> as a native NSMenu owned by another process, so the
-- menu is not reachable as `menu 1 of pop up button` and keystrokes must be
-- sent outside the `tell process` scope: open the pop-up, type the label
-- (type-select), press Return. Prints the pop-up's value afterwards.
-- Usage: osascript ax_select.applescript <process name> <popupIndex> <labelPrefix>
on run argv
  set theName to item 1 of argv
  set idx to (item 2 of argv) as integer
  set wanted to item 3 of argv
  tell application "System Events"
    tell process theName
      set frontmost to true
      set pops to my findPopups(window 1, 0)
      if (count of pops) < idx then error "pop-up button " & idx & " not found"
      click (item idx of pops)
    end tell
    delay 0.8
    -- Other apps (the Simulator after an app relaunch) may have taken
    -- focus meanwhile; the menu belongs to the frontmost app, so re-assert.
    tell process theName to set frontmost to true
    delay 0.2
    keystroke wanted
    delay 0.5
    keystroke return
    delay 0.5
    tell process theName
      set pops to my findPopups(window 1, 0)
      set v to ""
      try
        set v to value of (item idx of pops) as text
      end try
      return v
    end tell
  end tell
end run

on findPopups(e, depth)
  set found to {}
  if depth > 40 then return found
  tell application "System Events"
    try
      if role of e is "AXPopUpButton" then set end of found to e
      if role of e is not "AXTextArea" then
        repeat with c in UI elements of e
          set found to found & my findPopups(c, depth + 1)
        end repeat
      end if
    end try
  end tell
  return found
end findPopups
