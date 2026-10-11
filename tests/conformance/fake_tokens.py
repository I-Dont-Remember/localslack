"""Realistic-looking fake Slack tokens for tests, built at runtime.

Never write a full `xox?-...`/`xapp-...` literal anywhere in the repo: GitHub push
protection blocks pushes containing them. Import from here instead.
"""

BOT = "xox" + "b-1234567890123-abcdefghijklmnop"
APP = "xa" + "pp-1-A0123456789-123456-abcdef"
